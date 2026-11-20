# 编译说明（MSYS2 / MinGW-w64，Windows x64）

实测环境：Windows 11，`D:\app\MSYS2`，gcc 16.2.0，产物 `retroarch.exe` 约 18.2 MB。

## 1. 安装依赖

全国内网络下 pacman 只有南京镜像可用，先改 `/etc/pacman.d/mirrorlist.mingw64` 指向
`https://mirror.nju.edu.cn/msys2/mingw/x86_64/`，然后：

```sh
pacman -S --needed \
  mingw-w64-x86_64-gcc \
  mingw-w64-x86_64-make \
  mingw-w64-x86_64-pkgconf \
  mingw-w64-x86_64-glslang \
  mingw-w64-x86_64-spirv-cross \
  mingw-w64-x86_64-vulkan-headers \
  mingw-w64-x86_64-vulkan-loader \
  mingw-w64-x86_64-zlib \
  mingw-w64-x86_64-libpng \
  mingw-w64-x86_64-freetype
```

`glslang` / `spirv-cross` / `vulkan-*` 是 slang 着色器管线（`--enable-slang`）必需的，缺了 configure 会直接关掉 slang 支持。

## 2. configure

在 MSYS2 MinGW64 shell 里，源码根目录执行：

```sh
./configure \
  --disable-ffmpeg \
  --disable-ssa \
  --enable-vulkan \
  --enable-slang \
  --enable-glslang \
  --enable-spirv_cross
```

关掉 ffmpeg（录屏/视频用不到，还省一堆依赖）和 ssa（字幕渲染，同样用不到）。

## 3. 编译

```sh
make -j8
```

产物在源码根目录 `retroarch.exe`。

## 4. 注意

- 编译前先 `git apply` 补丁；`Makefile` 的 `-municode` 改动会影响入口点，没打补丁直接编译不会报错但中文路径依旧会乱码。
- `-DGAMEROOM_UNICODE_ARGV` 与 `retroarch.c` 里的 `wmain` 是配对的，只改一处会链接失败。
- 运行需要把 `cores\`、`system\`、`shaders\` 拷到 exe 同级目录；核心 DLL 依赖 RetroArch 根目录的 mingw 运行时（`libbz2`、`libfreetype`、`libgcc_s` 等），GameRoom 侧靠 `SetDllDirectoryW` 指向核心父目录解决。
