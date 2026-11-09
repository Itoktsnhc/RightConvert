package convert

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rightconvert/rightconvert/internal/rules"
)

func TestParseEncoders(t *testing.T) {
	out := []byte(`Encoders:
 V..... = Video
 ------
 V....D libx264              libx264 H.264 / AVC / MPEG-4 AVC
 A....D aac                  AAC (Advanced Audio Coding)
 A....D libmp3lame           libmp3lame MP3
`)
	enc := parseEncoders(out)
	for _, e := range []string{"libx264", "aac", "libmp3lame"} {
		if !enc[e] {
			t.Errorf("missing %s in %v", e, enc)
		}
	}
	if enc["="] || enc["Video"] || len(enc) != 3 {
		t.Errorf("legend parsed as encoders: %v", enc)
	}
}

func TestExpand(t *testing.T) {
	in := filepath.Join("C:", "My Music", "song one.flac")
	got := Expand([]string{"-i", "{input}", "-metadata", "title={name}", "{dir}", "{output}"}, in, "out.mp3")
	want := []string{"-i", in, "-metadata", "title=song one", filepath.Dir(in), "out.mp3"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("Expand = %q, want %q", got, want)
	}
}

func testRule(id, to string, from ...string) *rules.Rule {
	return &rules.Rule{ID: id, Label: id, From: from, To: to, Args: []string{"-i", "{input}", "{output}"}}
}

func touch(t *testing.T, p string) {
	t.Helper()
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestQueueUniqueOutputs(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "a.wav")
	touch(t, in)
	touch(t, filepath.Join(dir, "a.mp3"))
	block := make(chan struct{})
	q := NewQueue(func(ctx context.Context, r *rules.Rule, input, output string, p func(float64)) error {
		<-block
		return os.WriteFile(output, []byte("x"), 0o644)
	}, 4, nil)
	r := testRule("to-mp3", "mp3", "wav")
	j1, err := q.Add(r, in)
	if err != nil {
		t.Fatal(err)
	}
	j2, _ := q.Add(r, in)
	close(block)
	q.Wait()
	if filepath.Base(j1.Output) != "a (2).mp3" || filepath.Base(j2.Output) != "a (3).mp3" {
		t.Errorf("outputs %q %q", j1.Output, j2.Output)
	}
	for _, j := range q.Snapshot() {
		if j.State != Done {
			t.Errorf("job %d: %v %v", j.ID, j.State, j.Err)
		}
	}
}

func TestQueueRejects(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "a.mp3")
	touch(t, in)
	q := NewQueue(nil, 1, nil)
	if _, err := q.Add(testRule("to-mp3", "mp3", "wav", "mp3"), in); err == nil {
		t.Error("converted mp3 to mp3")
	}
	if _, err := q.Add(testRule("to-wav", "wav", "flac"), in); err == nil {
		t.Error("accepted an extension not in from")
	}
	if _, err := q.Add(testRule("to-wav", "wav", "mp3"), filepath.Join(dir, "missing.mp3")); err == nil {
		t.Error("accepted a missing file")
	}
}

func TestQueueConcurrencyAndCancel(t *testing.T) {
	dir := t.TempDir()
	var mu sync.Mutex
	running, peak := 0, 0
	release := make(chan struct{})
	q := NewQueue(func(ctx context.Context, r *rules.Rule, input, output string, p func(float64)) error {
		mu.Lock()
		running++
		peak = max(peak, running)
		mu.Unlock()
		defer func() { mu.Lock(); running--; mu.Unlock() }()
		select {
		case <-release:
			return errors.New("boom")
		case <-ctx.Done():
			return ctx.Err()
		}
	}, 2, nil)
	r := testRule("to-mp3", "mp3", "wav")
	var ids []int
	for i := range 5 {
		in := filepath.Join(dir, string(rune('a'+i))+".wav")
		touch(t, in)
		j, err := q.Add(r, in)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, j.ID)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := running
		mu.Unlock()
		if n == 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	q.Cancel(ids[4]) // still queued
	q.Cancel(ids[0]) // running
	close(release)
	q.Wait()
	if peak > 2 {
		t.Errorf("peak concurrency %d, want 2", peak)
	}
	snap := q.Snapshot()
	if snap[0].State != Canceled || snap[4].State != Canceled {
		t.Errorf("states %v %v, want canceled", snap[0].State, snap[4].State)
	}
	for _, j := range snap[1:4] {
		if j.State != Failed || j.Err == nil {
			t.Errorf("job %d: %v %v, want failed", j.ID, j.State, j.Err)
		}
	}
	q.ClearFinished()
	if len(q.Snapshot()) != 0 || q.Active() {
		t.Error("ClearFinished left jobs")
	}
}

// Integration tests with the real ffmpeg, when there is one.

func realFFmpeg(t *testing.T) *FFmpeg {
	t.Helper()
	if testing.Short() {
		t.Skip("short")
	}
	f, err := Locate(context.Background(), os.Getenv("RIGHTCONVERT_FFMPEG"), "")
	if err != nil {
		t.Skip(err)
	}
	return f
}

func generate(t *testing.T, f *FFmpeg, path string, args ...string) {
	t.Helper()
	all := append([]string{"-hide_banner", "-loglevel", "error", "-y"}, args...)
	if out, err := exec.Command(f.Path, append(all, path)...).CombinedOutput(); err != nil {
		t.Fatalf("generating %s: %v\n%s", path, err, out)
	}
}

func TestBuiltinRulesWithFFmpeg(t *testing.T) {
	f := realFFmpeg(t)
	dir := t.TempDir()
	sources := map[string]string{}
	add := func(name string, args ...string) {
		p := filepath.Join(dir, "src "+name) // a space, as real paths have
		generate(t, f, p, args...)
		sources[strings.TrimPrefix(filepath.Ext(name), ".")] = p
	}
	sine := []string{"-f", "lavfi", "-i", "sine=frequency=440:duration=2"}
	add("tone.wav", sine...)
	add("tone.flac", sine...)
	if f.HasEncoder("libx264") {
		add("clip.mov", append([]string{"-f", "lavfi", "-i", "testsrc=size=160x120:rate=15:duration=2"},
			append(sine, "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest")...)...)
		add("clip.mp4", append([]string{"-f", "lavfi", "-i", "testsrc=size=160x120:rate=15:duration=2"},
			append(sine, "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest")...)...)
	}
	for _, r := range rules.Builtin() {
		if m := f.Missing(r); len(m) > 0 {
			t.Logf("%s: skipped, ffmpeg lacks %v", r.ID, m)
			continue
		}
		var src string
		for _, ext := range []string{"flac", "wav", "mov", "mp4"} {
			if sources[ext] != "" && r.Accepts(ext) {
				src = sources[ext]
				break
			}
		}
		if src == "" {
			continue
		}
		t.Run(r.ID, func(t *testing.T) {
			ext := r.To
			if r.AutoTo() {
				var err error
				if ext, err = f.AudioExt(context.Background(), src); err != nil {
					t.Fatal(err)
				}
			}
			out := filepath.Join(t.TempDir(), "out."+ext)
			last := -2.0
			err := f.Run(context.Background(), r, src, out, func(p float64) { last = p })
			if err != nil {
				t.Fatal(err)
			}
			if last != 1 {
				t.Errorf("last progress %v", last)
			}
			if d := f.Duration(context.Background(), out); d < 1500*time.Millisecond {
				t.Errorf("output lasts %v", d)
			}
			if _, err := os.Stat(partName(out)); !errors.Is(err, os.ErrNotExist) {
				t.Error("temporary file left behind")
			}
		})
	}
}

func TestRunFailureCleansUp(t *testing.T) {
	f := realFFmpeg(t)
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.wav")
	if err := os.WriteFile(bad, []byte("not audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "bad.flac")
	err := f.Run(context.Background(), testRule("x", "flac", "wav"), bad, out, nil)
	if err == nil || !strings.HasPrefix(err.Error(), "ffmpeg: ") {
		t.Fatalf("err = %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("left files behind: %v", entries)
	}
}

func TestExtForAudioCodec(t *testing.T) {
	for codec, want := range map[string]string{
		"flac": "flac", "aac": "m4a", "alac": "m4a", "mp3": "mp3", "opus": "opus",
		"vorbis": "ogg", "pcm_s16le": "wav", "pcm_s24le": "wav", "pcm_s16be": "mka",
		"ac3": "ac3", "truehd": "mka",
	} {
		if got := ExtForAudioCodec(codec); got != want {
			t.Errorf("ExtForAudioCodec(%q) = %q, want %q", codec, got, want)
		}
	}
}

func TestQueueAutoExtension(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "clip.mp4")
	touch(t, in)
	touch(t, filepath.Join(dir, "clip-audio.flac"))
	q := NewQueue(func(ctx context.Context, r *rules.Rule, input, output string, p func(float64)) error {
		return os.WriteFile(output, nil, 0o644)
	}, 1, nil)
	q.ResolveExt = func(context.Context, string) (string, error) { return "flac", nil }
	r := &rules.Rule{ID: "x", Label: "x", From: []string{"mp4"}, To: rules.AutoExt, Suffix: "-audio",
		Args: []string{"-i", "{input}", "{output}"}}
	j, err := q.Add(r, in)
	if err != nil {
		t.Fatal(err)
	}
	if j.Output != "" {
		t.Errorf("output named before the format is known: %q", j.Output)
	}
	q.Wait()
	s := q.Snapshot()[0]
	if s.State != Done || filepath.Base(s.Output) != "clip-audio (2).flac" {
		t.Errorf("job = %v %q %v", s.State, s.Output, s.Err)
	}

	q.ResolveExt = func(context.Context, string) (string, error) { return "", errors.New("the file has no audio stream") }
	q.Add(r, in)
	q.Wait()
	if s := q.Snapshot()[1]; s.State != Failed || s.Output != "" {
		t.Errorf("job without audio = %v %q", s.State, s.Output)
	}
}

// The case that failed with "Could not find tag for codec flac": a video
// whose audio is FLAC, extracted without re-encoding.
func TestExtractFLACFromMP4(t *testing.T) {
	f := realFFmpeg(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "03.双黄蛋.mp4.mp4")
	generate(t, f, src, "-f", "lavfi", "-i", "testsrc=size=160x120:rate=15:duration=2",
		"-f", "lavfi", "-i", "sine=duration=2", "-c:v", "libx264", "-pix_fmt", "yuv420p",
		"-c:a", "flac", "-shortest")
	r := (&rules.Set{Rules: rules.Builtin()}).Get("extract-audio")
	q := NewQueue(f.Run, 1, nil)
	q.ResolveExt = f.AudioExt
	if _, err := q.Add(r, src); err != nil {
		t.Fatal(err)
	}
	q.Wait()
	j := q.Snapshot()[0]
	if j.State != Done {
		t.Fatalf("%v: %v", j.State, j.Err)
	}
	if filepath.Base(j.Output) != "03.双黄蛋.mp4-audio.flac" {
		t.Errorf("output %q", j.Output)
	}
	if codec, err := f.AudioCodec(context.Background(), j.Output); codec != "flac" {
		t.Errorf("output codec %q, %v", codec, err)
	}
}
