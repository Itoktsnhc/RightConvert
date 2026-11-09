# Changelog

发布时，release workflow 会取出与 tag 对应版本的章节作为 release notes。

## 0.1.0

RightConvert 的第一个版本：在资源管理器里右键音视频文件，选择“转换为 ▸”即可用 ffmpeg 转换。

### 功能

- **右键菜单**：为 FLAC、WAV、MP3、M4A、OGG、Opus、MP4、MOV、MKV、WebM、AVI 等格式添加“转换为”子菜单，每种格式只显示适用的规则。Windows 11 下在“显示更多选项”里。
- **内置规则**：MP3（320k / VBR V0）、FLAC、WAV、M4A（AAC）、OGG、Opus、MP4 / MOV（H.264 + AAC）、MKV / MP4 重新封装，以及不重新编码的“提取音频”。“提取音频”会按音轨格式自动选择扩展名，例如视频里的 FLAC 音轨会输出为 `.flac`。
- **自定义规则**：在 `%APPDATA%\RightConvert\rules.toml` 中覆盖、隐藏或新增规则；新增规则可以用 `to = "auto"`。
- **多选批量转换**：同时选中多个文件转换时，所有文件会进入同一个进度窗口，可以查看进度、取消和打开输出位置。
- **完成后保持窗口打开**：进度窗口中的勾选框，默认勾选。取消勾选后，全部成功时窗口会自动关闭；有失败时保持打开。转换成功的任务只有点“清除已完成”才会移除。
- **安全输出**：输出文件放在源文件旁边，不会覆盖已有文件，重名时自动加 `(2)` 这样的编号；转换失败或取消时不留下残缺文件。
- **不需要管理员权限**：安装包只为当前用户安装，右键菜单只写入 `HKEY_CURRENT_USER`；卸载时会一并移除右键菜单。

### 下载

- `rightconvert-setup-*-amd64.exe`：安装包，适用于大多数电脑。
- `rightconvert-setup-*-arm64.exe`：ARM 版 Windows 的安装包。
- `RightConvert-*-portable.zip`：免安装版；在设置的“右键菜单”页点“应用到右键菜单”即可添加菜单。
- `SHA256SUMS.txt`：校验和。

### 注意

- 需要 [ffmpeg](https://www.gyan.dev/ffmpeg/builds/)。程序会依次在设置中填写的路径、程序目录、`PATH` 中查找；和 ffmpeg 放在一起的 ffprobe 用于显示进度和识别音轨格式。
- 程序没有代码签名，首次运行时 SmartScreen 可能会提示，点“更多信息 → 仍要运行”即可。
