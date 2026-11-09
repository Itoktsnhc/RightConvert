# RightConvert

在资源管理器里右键音视频文件，选“转换为 ▸ MP3 / FLAC / WAV / M4A / MP4 / MOV …”即可转换。程序基于 [MyGo](https://github.com/egoist/mygo) 原生 UI，转换交给 ffmpeg 完成。

- **不需要管理员权限**：安装包只为当前用户安装到 `%LOCALAPPDATA%\Programs\RightConvert`；右键菜单只写入 `HKEY_CURRENT_USER\Software\Classes\SystemFileAssociations`。
- **多选文件**：资源管理器会给每个选中的文件单独启动一个进程，这些进程会合并到同一个队列窗口里处理。
- **不覆盖已有文件**：输出文件放在源文件旁边，重名时自动改成 `name (2).mp3`。转换过程中先写入 `.rcpart` 临时文件，成功后再改名；失败或取消时不留下残缺文件。

## ffmpeg

按以下顺序查找：设置里填写的路径 → 程序目录（及其下的 `bin`、`ffmpeg\bin`）→ `PATH`。如果当前 ffmpeg 缺少某条规则需要的编码器（例如 libmp3lame），这条规则不会出现在右键菜单里。

## 规则

内置规则见 [internal/rules/defaults.toml](internal/rules/defaults.toml)。自定义规则写在 `%APPDATA%\RightConvert\rules.toml`（在设置窗口中点“编辑 rules.toml”会自动创建）：

```toml
# 覆盖内置规则中的某些字段
[[rule]]
id   = "to-mp3"
args = ["-i", "{input}", "-map", "0:a:0", "-c:a", "libmp3lame", "-b:a", "192k", "{output}"]

# 隐藏一条内置规则
[[rule]]
id      = "to-ogg"
enabled = false

# 新增规则
[[rule]]
id       = "to-mp3-128"
label    = "MP3 (128k)"
from     = ["flac", "wav", "m4a"]
to       = "mp3"
suffix   = "-128k"
encoders = ["libmp3lame"]
args     = ["-i", "{input}", "-c:a", "libmp3lame", "-b:a", "128k", "{output}"]
```

`args` 中可用的占位符：`{input}`、`{output}`、`{dir}`、`{name}`。修改规则后，在设置中点“应用到右键菜单”才会生效。

## 命令行

```
RightConvert.exe                                  打开设置窗口
RightConvert.exe convert --rule to-mp3 a.flac …   转换指定文件
RightConvert.exe --register | --unregister        写入或移除右键菜单（安装包会调用）
```

## 开发

需要 Go 1.27.3 或更高版本。

```bash
go test ./...                 # 包含使用真实 ffmpeg 的集成测试（找不到 ffmpeg 时跳过）
go tool mygo dev              # 开发模式运行
go tool mygo build            # 生成 build/windows-amd64/ 下的 exe 和安装包
go run ./tools/genicon        # 重新生成 resources/icon.png
```
