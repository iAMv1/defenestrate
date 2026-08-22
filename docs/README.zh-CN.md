# DEFENESTRATE 🧹（中文）

**一个终端二进制文件，完成 Windows 的深度清理、卸载、磁盘分析与实时监控。**
命令行里的 CleanMyMac × AppCleaner × DaisyDisk × iStat——带完整交互式 TUI。
灵感来自 [tw93/mole](https://github.com/tw93/mole)（macOS 版）。

```
DEFENESTRATE                     交互式菜单
DEFENESTRATE clean --dry-run     预览可回收的缓存空间
DEFENESTRATE uninstall           可搜索列表 → 证据计划 → 执行
DEFENESTRATE analyze C:\         磁盘树状图 · 大文件 · 回收站删除
DEFENESTRATE status              实时 CPU / 内存 / 磁盘仪表盘
DEFENESTRATE optimize            有边界的维护任务
DEFENESTRATE hud                 系统托盘实时小组件
DEFENESTRATE update              自更新
DEFENESTRATE history --json      操作审计日志
```

## 模式

| 模式 | 功能 |
|---|---|
| **clean** | 系统临时文件、Windows 更新缓存、传递优化、崩溃转储、旧 CBS 日志、缩略图、Chromium/Firefox 浏览器缓存、开发包缓存（npm/pip/yarn/NuGet） |
| **uninstall** | 注册表程序 + 商店应用：厂商静默卸载器或 `Remove-AppxPackage`，随后回收注册表证据位置 |
| **analyze** | 并发遍历（16 worker）、容量条形图、≥64MB 大文件、`--json`、`--delete` 走回收站 |
| **status** | 健康评分仪表盘：CPU 总量+每核、内存、各磁盘读写速率；60 秒迷你趋势图 |
| **optimize** | 有边界维护任务：DNS/ARP 刷新、回收站清空（需确认）、搜索服务重启等 |
| **hud** | 系统托盘：CPU / 内存实时显示 |

## 分类法则

**DEFENESTRATE 绝不删除它无法分类的东西。** 每个文件都有裁决：

- `KnownJunk` — 明确的可再生扩展名（`.tmp .log .dmp…`）→ 可自动回收
- `Executable` — `.exe .dll .msi .iso .ps1…` → **仅标记待审**
- `Unknown` — 无扩展名/未知格式 → **仅标记待审**

存疑即 Unknown，Unknown 永不自动删除。

## 安全模型

- 全局 `--dry-run`：只预览路径与大小，不写任何数据
- 只进回收站：不存在永久删除代码路径
- 守卫清单：`%WINDIR%`、Program Files、ProgramData 与用户配置根目录拒绝删除；
  显式安全区（如 `Windows\Temp`）仅清理内容
- 三态进程守卫：运行中跳过；枚举失败 = 拒绝清理
- 凭据保护：`.ssh`、密码管理器、浏览器 Login Data 即使在合格目录内也拒绝
- 操作日志：所有变更写入 `%LOCALAPPDATA%\DEFENESTRATE\operations.log`

## 无硬编码

- 清理目标全部来自环境变量或注册表枚举（`internal/rules/rules.go` 为纯数据）
- 版本号由 `-ldflags -X main.version=…` 注入
- 发布流水线从 `go.mod` 取工具链、从 tag 取产物名

## 构建

```powershell
go build -trimpath -ldflags "-s -w -X main.version=$(git describe --tags --always)" -o DEFENESTRATE.exe
```

需要 Go ≥ 1.22。MIT 许可。更多语言文档欢迎 PR。
