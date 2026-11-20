# RetroArch 1.17.0 改造补丁（GameRoom 用）

这是 GameRoom 内嵌模式阶段对 RetroArch 源码做的**全部改动**，共 3 个文件、86 行插入：

| 文件 | 改动 |
| --- | --- |
| `Makefile` | `-mwindows -municode -DGAMEROOM_UNICODE_ARGV`，让中文路径下的 argv 正常 |
| `gfx/common/win32_common.c` | 读环境变量 `GAMEROOM_PARENT_HWND`，把主窗口建成 `WS_CHILD`、不 `ShowWindow`/`SetForegroundWindow` |
| `retroarch.c` | 加 `wmain`，把宽字符 argv 转成 UTF-8 |

> 现在的正式方案是 GameRoom 自己当 libretro 宿主（进程内加载核心 DLL），已经**不再启动 RetroArch 进程**，所以这个补丁只在需要复用 RetroArch 窗口行为时才有意义。补丁仍然完整保留，作为历史存档。

## 文件

- `retroarch-gameroom-window.patch` —— 完整 unified diff
- `build.md` —— 编译步骤、依赖、参数

## 基线

- 仓库：`https://github.com/libretro/RetroArch`
- 版本：v1.17.0
- 提交：`ad8975c`（Fetch translations from Crowdin）

## 使用

```sh
git clone https://github.com/libretro/RetroArch.git retroarch
cd retroarch
git checkout ad8975c
git apply /path/to/retroarch-gameroom-window.patch
```

然后按 `build.md` 在 MSYS2 里编译。

## 源码归档

source/retroarch-1.17.0-gameroom-src.7z 是打过补丁后的完整源码树（不含 .git 与编译中间产物，45 MB），7-Zip 解出即可编译。
