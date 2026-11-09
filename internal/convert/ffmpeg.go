// Package convert runs ffmpeg for conversion rules.
package convert

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/rightconvert/rightconvert/internal/rules"
)

// FFmpeg is a located ffmpeg, with the encoders it was built with.
type FFmpeg struct {
	Path     string
	Probe    string // ffprobe, or "" when it was not found beside ffmpeg
	Version  string
	encoders map[string]bool
}

// ErrNotFound is returned by Locate when no ffmpeg was found.
var ErrNotFound = errors.New("ffmpeg not found: install it, put ffmpeg.exe beside RightConvert.exe, or set its path in Settings")

func exeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// Candidates lists where Locate looks for ffmpeg, in order: the configured
// path (a file or a directory), the app's directory and its bin and
// ffmpeg\bin subdirectories, then PATH.
func Candidates(configured, appDir string) []string {
	var c []string
	if configured != "" {
		if fi, err := os.Stat(configured); err == nil && fi.IsDir() {
			c = append(c, filepath.Join(configured, exeName("ffmpeg")), filepath.Join(configured, "bin", exeName("ffmpeg")))
		} else {
			c = append(c, configured)
		}
	}
	if appDir != "" {
		for _, sub := range []string{"", "bin", filepath.Join("ffmpeg", "bin")} {
			c = append(c, filepath.Join(appDir, sub, exeName("ffmpeg")))
		}
	}
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		c = append(c, p)
	}
	return c
}

// Locate finds ffmpeg (see Candidates) and asks it for its encoders.
func Locate(ctx context.Context, configured, appDir string) (*FFmpeg, error) {
	for _, p := range Candidates(configured, appDir) {
		if fi, err := os.Stat(p); err != nil || fi.IsDir() {
			continue
		}
		f := &FFmpeg{Path: p}
		if err := f.load(ctx); err != nil {
			continue
		}
		probe := filepath.Join(filepath.Dir(p), exeName("ffprobe"))
		if _, err := os.Stat(probe); err == nil {
			f.Probe = probe
		} else if lp, err := exec.LookPath("ffprobe"); err == nil {
			f.Probe = lp
		}
		return f, nil
	}
	return nil, ErrNotFound
}

func (f *FFmpeg) load(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := command(ctx, f.Path, "-hide_banner", "-encoders").Output()
	if err != nil {
		return err
	}
	f.encoders = parseEncoders(out)
	if len(f.encoders) == 0 {
		return fmt.Errorf("%s lists no encoders", f.Path)
	}
	if v, err := command(ctx, f.Path, "-hide_banner", "-version").Output(); err == nil {
		line, _, _ := strings.Cut(string(v), "\n")
		f.Version = strings.TrimSpace(strings.TrimPrefix(line, "ffmpeg version "))
	}
	return nil
}

// parseEncoders reads the output of `ffmpeg -encoders`: a legend, a line of
// dashes, then a line per encoder of its flags and its name.
func parseEncoders(out []byte) map[string]bool {
	enc := map[string]bool{}
	started := false
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if !started {
			started = len(fields) == 1 && strings.Trim(fields[0], "-") == ""
			continue
		}
		if len(fields) >= 2 {
			enc[fields[1]] = true
		}
	}
	return enc
}

// HasEncoder reports whether ffmpeg was built with the encoder.
func (f *FFmpeg) HasEncoder(name string) bool { return f.encoders[name] }

// Missing returns the encoders the rule needs that ffmpeg lacks.
func (f *FFmpeg) Missing(r *rules.Rule) []string {
	var m []string
	for _, e := range r.Encoders {
		if !f.encoders[e] {
			m = append(m, e)
		}
	}
	return m
}

// Duration returns the length of a media file, or 0 when it is unknown.
func (f *FFmpeg) Duration(ctx context.Context, path string) time.Duration {
	if f.Probe == "" {
		return 0
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := command(ctx, f.Probe, "-v", "error", "-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1", path).Output()
	if err != nil {
		return 0
	}
	sec, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil || sec <= 0 {
		return 0
	}
	return time.Duration(sec * float64(time.Second))
}

// AudioCodec returns the codec of the first audio stream of a media file.
func (f *FFmpeg) AudioCodec(ctx context.Context, path string) (string, error) {
	if f.Probe == "" {
		return "", errors.New("ffprobe not found beside ffmpeg: it is needed to tell the audio format")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := command(ctx, f.Probe, "-v", "error", "-select_streams", "a:0",
		"-show_entries", "stream=codec_name", "-of", "default=noprint_wrappers=1:nokey=1", path).Output()
	if err != nil {
		return "", fmt.Errorf("ffprobe: %w", err)
	}
	codec := strings.TrimSpace(string(out))
	if codec == "" {
		return "", errors.New("the file has no audio stream")
	}
	return codec, nil
}

// AudioExt returns the extension of a file holding the first audio stream
// of path as it is, for rules whose To is "auto".
func (f *FFmpeg) AudioExt(ctx context.Context, path string) (string, error) {
	codec, err := f.AudioCodec(ctx, path)
	if err != nil {
		return "", err
	}
	return ExtForAudioCodec(codec), nil
}

// ExtForAudioCodec is the extension of the usual file of an audio codec,
// or mka, which holds any codec.
func ExtForAudioCodec(codec string) string {
	switch codec {
	case "aac", "alac":
		return "m4a"
	case "mp3", "flac", "opus", "ac3", "eac3", "dts":
		return codec
	case "vorbis":
		return "ogg"
	case "mp2":
		return "mp2"
	case "wmav1", "wmav2":
		return "wma"
	}
	// WAV holds little-endian PCM; big-endian PCM, as in MOV, does not fit.
	if strings.HasPrefix(codec, "pcm_") && (strings.HasSuffix(codec, "le") || codec == "pcm_u8") {
		return "wav"
	}
	return "mka"
}

// Expand replaces the placeholders of a rule's args.
func Expand(args []string, input, output string) []string {
	base := filepath.Base(input)
	r := strings.NewReplacer(
		"{input}", input,
		"{output}", output,
		"{dir}", filepath.Dir(input),
		"{name}", strings.TrimSuffix(base, filepath.Ext(base)),
	)
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = r.Replace(a)
	}
	return out
}

// Run converts input to output with the rule, reporting progress between 0
// and 1, or -1 while the length is unknown. The file is written under a
// temporary name and renamed when ffmpeg succeeds, so that a canceled or
// failed conversion leaves nothing behind.
func (f *FFmpeg) Run(ctx context.Context, r *rules.Rule, input, output string, progress func(float64)) error {
	if progress == nil {
		progress = func(float64) {}
	}
	total := f.Duration(ctx, input)
	tmp := partName(output)
	args := append([]string{"-hide_banner", "-nostdin", "-y", "-loglevel", "error", "-nostats", "-progress", "pipe:1"},
		Expand(r.Args, input, tmp)...)
	cmd := command(ctx, f.Path, args...)
	var stderr tailBuffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	if total <= 0 {
		progress(-1)
	}
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), "=")
		if !ok || total <= 0 {
			continue
		}
		// out_time_us is in microseconds; old builds misname it out_time_ms.
		if k == "out_time_us" || k == "out_time_ms" {
			if us, err := strconv.ParseInt(v, 10, 64); err == nil && us >= 0 {
				progress(min(1, float64(us)*float64(time.Microsecond)/float64(total)))
			}
		}
	}
	err = cmd.Wait()
	if ctx.Err() != nil {
		os.Remove(tmp)
		return ctx.Err()
	}
	if err != nil {
		os.Remove(tmp)
		if msg := stderr.String(); msg != "" {
			return fmt.Errorf("ffmpeg: %s", msg)
		}
		return fmt.Errorf("ffmpeg: %w", err)
	}
	if err := os.Rename(tmp, output); err != nil {
		os.Remove(tmp)
		return err
	}
	progress(1)
	return nil
}

// partName is the temporary name output is written under; it keeps the
// extension, from which ffmpeg picks the format.
func partName(output string) string {
	ext := filepath.Ext(output)
	return strings.TrimSuffix(output, ext) + ".rcpart" + ext
}

// tailBuffer keeps the last lines ffmpeg wrote to stderr.
type tailBuffer struct{ b []byte }

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.b = append(t.b, p...)
	if len(t.b) > 4096 {
		t.b = t.b[len(t.b)-4096:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	lines := strings.Split(strings.TrimSpace(string(t.b)), "\n")
	if len(lines) > 3 {
		lines = lines[len(lines)-3:]
	}
	return strings.TrimSpace(strings.Join(lines, " | "))
}
