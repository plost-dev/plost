<p align="center">
  <a href="https://plo.st"><img src="docs/assets/plost-logo.svg" width="240" alt="Plost"></a>
</p>
<h1 align="center">Plost Go</h1>
<p align="center">轻、快、省、美的网站访问统计，供个人自托管。</p>
<p align="center">
  <a href="https://go.dev/"><img alt="Go 1.27" src="https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white"></a>
  <a href="https://www.sqlite.org/"><img alt="SQLite" src="https://img.shields.io/badge/SQLite-Embedded-003B57?logo=sqlite&logoColor=white"></a>
  <a href="https://react.dev/"><img alt="React 19" src="https://img.shields.io/badge/React-19-149ECA?logo=react&logoColor=white"></a>
  <a href="https://www.typescriptlang.org/"><img alt="TypeScript 7" src="https://img.shields.io/badge/TypeScript-7-3178C6?logo=typescript&logoColor=white"></a>
  <a href="https://vite.dev/"><img alt="Vite 8" src="https://img.shields.io/badge/Vite-8-646CFF?logo=vite&logoColor=white"></a>
  <a href="LICENSE"><img alt="MIT License" src="https://img.shields.io/badge/License-MIT-22C55E"></a>
</p>

Plost 是基于 Go + SQLite 的轻量网站访问统计工具。一个可执行文件内嵌完整界面、CSS、JavaScript、字体和追踪脚本，运行时不需要 Node、Bun、Go 工具链、Docker 或外部数据库。

## 开始使用

从源码构建需要 Go 1.27 或更新版本。仓库包含已构建的网页资源，可直接编译：

```sh
go build -trimpath -ldflags='-s -w' -o plost ./cmd/plost
./plost -listen 127.0.0.1:3101 -data ./data
```

打开 `http://127.0.0.1:3101`，创建唯一的实例管理员，然后添加网站。首次初始化完成后，公开注册永久关闭。一个管理员可以管理多个网站，也可以为不同服务器分别部署独立实例。

生产环境使用对应 Linux 架构的预编译程序，在 HTTPS 反向代理后运行：

```sh
./plost -listen 127.0.0.1:3101 -data /var/lib/plost -origin https://stats.example.com
```

`-origin` 固定管理和采集服务的实际域名，并为登录 Cookie 启用 Secure。应使用 HTTPS，不向公网暴露首次初始化中的空实例。反向代理示例见 [deploy/Caddyfile](deploy/Caddyfile)。

管理员账号和统计都保存在数据目录。程序启动时自动创建数据库，数据目录权限为 0700、数据库文件权限为 0600。替换程序升级时保持相同的 `-data` 目录，升级前从后台下载备份。

## 保留的内容

- 六项只读汇总：访客、访问次数、浏览量、每次访问浏览量、跳出率和访问时长；前三项同时绘制三条曲线，另有近 5 分钟在线人数。
- 热门页面、流量来源、国家和地区。
- 浏览器、操作系统、设备、语言与屏幕分辨率。
- 原版时间选择器、趋势缩放和访问时段热力图；按来源、页面及设备等维度筛选。
- 多站点、公开统计、可撤销的只读分享、统计 API 与 CSV 报表导出。
- 匿名近期访问记录，不展示原始 IP。
- 中文和英文、深浅色模式、四种强调色、手机界面。
- 账号密码、验证器两步验证、一次性恢复码，以及 Passkey（指纹、面容或安全密钥）登录。
- 后台创建并下载加密完整备份，上传校验、完整恢复和恢复前自动保护。
- 自动备份：按小时、天或周定时生成加密备份，按份数保留在数据目录，可选上传到 Cloudflare R2 或 Amazon S3 兼容存储，并能从本地或云端列表恢复。
- 站点检查：从服务器侧查看网站是否返回 HTTP 200，以及 HTTPS 证书和域名何时到期；站点页工具栏显示「HTTP 200 · SSL N 天」，站点列表显示证书剩余天数。

按照新的简化定位，不移植企业官网、个人博客、电商等场景模板，也不加入模板转化报表、漏斗、留存、AI SEO、搜索集成，以及服务器资源监控、探针和多节点探测。站点检查只看网站本身是否可用以及证书和域名的到期时间。自定义事件、文件下载和外链点击保留轻量计数。

## 网站接入

将域名替换成自己的服务和网站，在网站的 `<head>` 中加入：

```html
<script defer data-domain="example.com" src="https://stats.example.com/script.js"></script>
```

后台的网站设置会自动生成实际地址。保留 `/js/script.js`、`/tracker.js`、`window.plost`、`window.litestats` 和 `litestats:visitor-id` 兼容。追踪脚本支持 SPA 页面切换、外链和文件下载事件。

采集会检查页面域名与站点匹配；浏览器跨域采集不携带 Cookie。匿名访客 ID 保存在被统计网站的 localStorage，服务器只保存按站点生成的 HMAC 摘要。原始 IP 不写入统计。页面只保存路径和单独的 UTM 字段，不保存整段查询参数；自定义事件属性由调用方负责避免发送敏感内容。不宣称“不使用 Cookie 就自动免除所有同意要求”。

地图沿用原版，后台可配置 Mapbox 公开 Token，无 Token 时使用内嵌备用地图。当前采集只提供国家，不采集城市或省州，相关视图为空。国家默认未知。若反向代理可信地覆盖客户端 IP 和 `CF-IPCountry`，且从本机回源，可以用 `-trust-proxy` 接受这些头。该选项只信任回环地址上的直接代理；没有启用外部 IP 查询服务，不会悄悄向第三方发送访客 IP。

## 统计口径

- **访客**：所选时段内产生 pageview 的不同匿名访客。
- **浏览量**：pageview 事件数量，重复访问会累加。
- **访问次数**：同一访客连续活动视为一次访问，超过 30 分钟无活动后重新计次。
- **在线人数**：近 5 分钟产生过事件的匿名访客，不是 WebSocket 连接数或持续心跳。
- **跳出率**：时段内仅有一个 pageview 的访问占比。
- **平均访问时长**：每次访问的最后与第一个 pageview 的时间差；没有心跳，最后一页停留不能完整测量。
- **来源及设备报表**：按不同访客计数；同一访客可能跨组，组内百分比用当前列表的分组计数之和作分母。原版界面的热门页面显示不同访客，基础报表 API 的 `pages` 字段按 pageview 计数，`ui=1` 的 `details.pageVisitors` 提供访客口径。
- **趋势对比**：与紧邻的等长前一时段比较；“昨天”按上一完整自然日比较。全部时间不做环比。
- **时区**：按每个站点配置生成日期边界，趋势分桶支持夏令时。

请求返回 202 前，事件已经在 SQLite 事务中提交。采集通过有界队列和小批次事务串行写入；队列满时返回明确的 503 `event_buffer_full`，兼容脚本的有限重试。

## 统计 API

在站点设置创建分享链接，管理员可再次查看完整链接；分享报告本身不返回凭证。凭证只能读取所属网站，不能访问账号、设置、近期活动或备份。

```sh
curl --fail-with-body 'https://stats.example.com/api/v1/stats?period=summary' \
  -H "Authorization: Bearer $PLOST_TOKEN"
```

保留原来的 `apiVersion/site/period/generatedAt/onlineWindowSeconds/data` 响应结构。支持 `summary`、`live`、`today`、`yesterday`；可选 `domain` 必须与凭证所属网站相同。每个凭证每分钟最多 60 次读取。实例内 `/docs/api` 提供说明，`/api/v1/openapi.json` 提供 OpenAPI 定义。

## Passkey 登录

在「设置 → 安全」添加 Passkey 后，登录页会出现「使用 Passkey 登录」，无需输入邮箱和密码。Passkey 使用 WebAuthn 的可发现凭据，注册和登录都要求用户验证（指纹、面容或设备 PIN），所以一次验证就等同于两个因素，登录后不再询问验证器代码。

- **访问地址**：浏览器只在 HTTPS 或 `localhost` 上提供 WebAuthn，并把 Passkey 绑定到域名。生产环境请通过 HTTPS 域名访问，并设置 `-origin` 为该域名；本机试用请打开 `http://localhost:3101`，不要用 `127.0.0.1`，IP 地址不是有效的 Passkey 域名。更换域名后，旧域名下创建的 Passkey 不能用于新域名。
- **添加和移除需要确认身份**：两者都要输入当前账号密码，被盗用的会话不能给自己添加一把登录方式。最多 20 把，名称可选。
- **服务端校验**：使用成熟的 `go-webauthn` 库验证来源、域名哈希、挑战和签名。挑战只在服务端内存里保存 5 分钟且只能使用一次，签名计数器没有前进（疑似凭据被复制）时拒绝登录。公钥凭据信息放在数据库，列表接口不返回凭据内容；它们会随完整备份一起保存和恢复。
- **没有 Passkey 时**：登录页不显示相关按钮，密码、两步验证和恢复码照常工作；Passkey 丢失或设备损坏时仍可用密码登录后移除。

## 站点检查

在站点设置里打开「站点检查」，Plost 的服务端会代你查看这个网站，不需要在网站上安装任何东西：

- **可用性**：每 5 分钟访问一次设定的路径（默认 `/`，必须在本站内），跟随不超过 3 次跳转，最终返回 HTTP 200 视为正常。记录状态码和响应时间，检查页显示最近 24 小时的响应曲线、成功率和响应中位数。同一站点一分钟内只会启动一次检查，手动「立即检查」也受此限制。
- **证书到期**：读取网站 HTTPS 证书的到期时间和验证级别（DV、OV 或 EV），并判断证书是否有效、域名是否匹配；证书过期或未通过验证时给出原因。
- **域名到期**：向该域名后缀的注册局查询 RDAP（先向 IANA 取得各后缀的 RDAP 服务地址，每天缓存一次），读取注册日期、到期日期和注册商。`.cn` 域名在 RDAP 没有结果时回退到 CNNIC 的官方 WHOIS。到期信息每天更新一次，临时失败后 15 分钟重试，失败时显示上次成功的日期并标明是旧数据。不使用任何第三方域名查询服务。
- **显示位置**：站点页工具栏的「HTTP 200 · SSL N 天」徽标、站点列表里的 SSL 剩余天数，以及「站点检查」页。这些信息只有登录后才能看到，分享链接和公开页面不包含它们。
- **数据**：检查结果保存在数据库里，保留 7 天，随完整备份一起保存和恢复。

**出站连接限制**：站点检查和云存储备份只连接公网地址。连接时检查实际解析到的地址，拒绝内网、回环、链路本地和保留地址，不跟随到这类地址的跳转，也不使用环境代理。某些本机代理工具（常见于开发用的 Mac）会把所有域名解析成 `198.18.0.0/15` 里的虚拟地址，这时所有检查都会显示「域名没有可安全访问的公网地址」。确认是这种情况时，用启动参数明确放行该网段：

```sh
./plost -listen 127.0.0.1:3101 -data ./data -allow-outbound 198.18.0.0/15
```

`-allow-outbound` 接受逗号分隔的网段，默认为空；比 /8（IPv6 为 /16）更宽的网段会拒绝启动。它只能由启动程序的人设置，浏览器无法改变，也只放宽地址检查，不影响对主机名和端口的限制。不要为了监控内网站点而随意放开。

**限制**：只检查 HTTPS 默认端口上的站点。没有提供 RDAP 的域名后缀只显示「暂不支持，请到注册商查看」。`.cn` 的 WHOIS 回退需要服务器能访问 TCP 43 端口。证书检查只读取该主机名返回的证书，不检查中间证书的有效期。检查只在 Plost 运行时进行，停机期间的检查不会补做。检查来自这一台服务器，没有多地点探测。

## 备份与恢复

打开「账号 → 备份与恢复」，输入当前账号密码和独立的备份密码。备份扩展名为 `.plostbk`，使用 scrypt 派生密钥及分块 AES-256-GCM 加密，并验证最终结束块，拒绝篡改和截断。密码不保存在备份旁，也不能找回，应与备份分别保存。

导出通过 SQLite `VACUUM INTO` 生成包含已提交 WAL 数据的一致性快照，不直接复制正在使用的数据库文件。加密按 64 KiB 分块处理，不将整个数据库加载到内存。

恢复前需要当前账号密码、备份密码和 `RESTORE` 确认。服务校验完整性、数据库版本、表结构及外键；暂停内部数据库访问，在 `data/backups/before-restore-*.db` 中保存恢复前快照，再切换新库。切换失败尝试回滚到旧快照；成功后撤销全部会话，使用备份中的账号重新登录。恢复前快照是权限受限的本地 SQLite 文件，应像生产数据库一样保护。

此版无需停止外部数据库或通过 Docker socket 换库，所以由 Go 进程内的互斥机制完成切换，不依赖原版独立的特权运维进程。

### 自动备份与云存储

在「账号 → 备份与恢复」的「自动备份」中选择每小时、每天或每周，并设置**计划备份密码**。服务每 30 秒检查一次，按服务器时区执行；错过的时间点会在服务恢复后补跑一次，失败后至少间隔 15 分钟再重试。备份文件与手动备份格式相同（`.plostbk`），保存在数据目录的 `backups/scheduled/`，只保留最新的若干份；保留份数设为 0 时只保留云端副本。「立即备份」在后台执行，页面会跟随进度并显示计划任务的结果。

- **凭据保存**：无人值守加密需要服务器保存计划备份密码。它与 S3 / R2 密钥一起用 AES-256-GCM 加密，密钥放在数据目录的 `secrets.key`（权限 0600），不放进数据库，所以备份文件和恢复前快照都不带着解锁凭据的密钥。复制或迁移数据目录时请连同 `secrets.key` 一起带走；在没有它的服务器上恢复后，已保存的凭据无法解密，设置页会提示重新填写，不影响服务运行。恢复备份时仍要输入创建它时的备份密码，请另外保管。
- **修改需要确认身份**：更改备份去向、密钥或计划备份密码时需要当前账号密码，避免被盗用的会话把加密备份重定向到别处。保存后接口和页面都不会返回密钥原文。
- **云存储**：使用自行实现的 AWS Signature V4，不依赖 AWS SDK。服务地址必须是公网 HTTPS：拒绝内网、回环和链路本地等地址，连接时再次检查实际解析到的地址，不跟随重定向，也不使用环境代理。已保存的密钥不会用于另一个服务或 Endpoint，更换时必须重新填写。「测试连接」只发一次受限的列表请求，上传权限在第一次备份时才会得到验证。
- **云端不自动删除**：程序只清理本地 `backups/scheduled/` 中符合命名规则的文件。云端旧备份请用存储桶的生命周期规则管理。
- **从列表恢复**：本地或云端列表里的备份与上传文件使用同一套解密、完整性与表结构校验和恢复前自动快照，仍需要当前账号密码、备份密码和 `RESTORE` 确认。
- **限制**：数据库超过 1 GiB 时，现有恢复流程无法读取，计划备份会明确报错，而不是生成恢复不了的文件。恢复回来的数据库不含之后的备份记录，所以每小时计划可能在恢复后立即补跑一次。

**旧版 PostgreSQL + ClickHouse 的 `.lsb` 备份不能直接恢复到 SQLite。** 本次没有读取或迁移旧实例，也没有移植旧备份加密协议。Umami 的历史数据可以用下面的搬迁工具导入；旧版 LiteStats 的历史数据仍需另行实现离线转换并核对指标，不覆盖原数据。旧账号/团队导入尚未移植；当前可使用密码、TOTP、Passkey，以及手动、计划和云端的加密完整备份。

## 从 Umami 搬迁

`plost-import-umami` 是独立的命令行程序（`./build.sh` 一并生成），把 Umami 的历史访问数据搬进 Plost。它只读取 Umami 的 PostgreSQL，不修改 Umami；只写入一个已停止的 Plost 实例；只带走 Plost 能表示的内容，其余在报告里按原因列出。它在单独的模块 `cmd/plost-import-umami` 中，PostgreSQL 驱动不会进入 Plost 服务程序。

| 搬迁 | 说明 |
| --- | --- |
| 站点 | 由你逐个指定 Umami 站点 ID 与 Plost 域名，不按名称猜测；没有指定的站点不会导入。可自动创建站点（默认私有） |
| 浏览与自定义事件 | 路径、标题、来源、UTM、主机名，以及自定义事件的属性（最多 32 项简单值） |
| 访客属性 | 国家、浏览器、系统、设备、屏幕分辨率、语言 |

不搬迁：账号、团队、报表、看板、分享链接、城市与地区、URL 查询参数（UTM 除外）、点击标识，以及自定义事件以外的事件类型（链接、像素、收入等，报告里按数量列出）。名称为空、与 `pageview` 冲突或超过 255 字节的自定义事件会被跳过并计数。

```sh
export UMAMI_DATABASE_URL='postgres://umami@db.example.com/umami?sslmode=require'
./plost-import-umami list
./plost-import-umami import -data /var/lib/plost -site <Umami站点ID>=example.com -create-sites -timezone Asia/Shanghai -until 2026-11-01T00:00:00Z
```

1. `list` 列出 Umami 里的站点和各类事件数量。
2. `import` 默认只做**预演**：完整读取并核对，不写入任何东西，Plost 在运行时也可以预演。
3. 确认后**先停止 Plost**，加上 `-apply` 再运行。程序会先在 `backups/before-import-*.db` 保存快照（本次没有任何新增时自动删除），写入后在 `backups/` 生成核对清单。
4. 启动 Plost 查看。需要撤销时运行 `plost-import-umami rollback -data … -site example.com -apply`，它只删除从 Umami 导入的事件，不影响 Plost 自己采集的数据。

**对账**：写入在同一个事务里完成。对每个站点按站点时区逐日比较浏览数、自定义事件数、访客数和访问数：一边是 Umami 数据库自己算出的结果，另一边是写入后在 Plost 里重新统计的结果，任何一项不一致整批回滚、什么都不留下。每个导入的事件带有来源编号 `umami:<站点ID>:<事件ID>` 并有唯一索引，所以重复运行不会重复导入，也可以随时补齐新增的数据。

**切换边界**：如果目标站点已经有 Plost 实时采集的数据，而导入范围与它重叠，会因重复计数而被拒绝。请用 `-until` 把结束时间设为把统计脚本换成 Plost 的时刻（`-since` 限定起点），确认无误后才可用 `-allow-overlap` 放行。

**口径**：

- 访客对应 Umami 的 `session_id`，访问对应 `visit_id`；没有访问编号的旧事件，每个访客算一次访问。Umami 的访客标识按月轮换，也与 Plost 新采集的标识不同，所以长时间范围的访客数不会与 Umami 逐位相同，切换前后的同一个人会算作两位访客；浏览量和事件数则逐条对应。
- 时间向下取整到秒。浏览器、系统、设备并入 Plost 自己的分类（例如 Samsung 或 Yandex 浏览器归为 Chrome，laptop 归为 Desktop），来源规则与实时采集相同：UTM 来源优先，其次引荐域名，本站引荐算 Direct。Plost 只记录国家。
- 只支持 PostgreSQL 版 Umami v2 的表结构，并要求 `website_event.visit_id` 列；其他版本会被明确拒绝，而不是猜测。

**安全**：Umami 连接地址只从环境变量读取，不接受命令行参数，也不会写入清单或日志；程序在只读、可重复读的事务里读取，所有查询看到同一时刻的数据；清单与快照权限为 0600。**这个工具只在合成数据和一次性的 PostgreSQL 上验证过，没有对真实的 Umami 数据运行过；对线上数据使用前请先备份 Umami 和 Plost，并先预演。**

修改主模块的依赖后，在 `cmd/plost-import-umami` 里运行 `go mod tidy`。它的集成测试需要一个一次性的临时 PostgreSQL：把 `UMAMI_TEST_DATABASE_URL` 指向空库，不设置时自动跳过。

## 原版界面与开发

界面直接复用原单人版的 CSS、图标、字体、地图和报表组件。趋势按用户要求改为三条曲线同时显示，六个汇总值保留且不做指标切换。API 文档入口位于设置侧栏，旧 `/docs/api` 地址仍可用。已删除的场景模板及业务报表不恢复。页脚采用后续更新的紧凑版本，两个 40px 图标区域相邻，Logo 与 GitHub 都为灰色，悬停白色。

修改前端时需要 Bun，完成后再编译 Go：

```sh
cd frontend
bun install --frozen-lockfile
bun run typecheck
bun run check:unused
bun run build
cd ..
go build -trimpath -ldflags='-s -w' -o plost ./cmd/plost
```

React、Vite 和 Tailwind 只用于开发构建，运行时全部由 Go 内嵌资源提供。先完成网页构建，再编译 Go。`routeTree.gen.ts` 由构建生成，不手动维护。来源清单见 [frontend/upstream-manifest.json](frontend/upstream-manifest.json)，对照方法见 [frontend/UI-PARITY.md](frontend/UI-PARITY.md)。

## 交付与资源

```sh
./build.sh
```

生成 macOS arm64、Linux amd64、Linux arm64 三个独立可执行文件，另有同样三个平台的 `plost-import-umami`，使用 `CGO_ENABLED=0`，无需目标主机安装 C 动态库或 Go 环境。源码按模块拆分维护；“单文件”指最终交付，不要求把所有源码塞进一个 `.go` 文件。

运行时仍会生成 `plost.db`、防止重复启动的 `plost.lock`，以及 SQLite 可能使用的 `plost.db-wal`、`plost.db-shm`；保存备份凭据时会创建 `secrets.key`，备份与恢复前快照放在 `backups/`。数据库和备份不嵌入程序。程序默认对 Go 运行时设置 64 MiB 软内存目标，并将 SQLite 页缓存限制在约 4 MiB。它不是整个进程的硬内存上限，不能当作内存占用承诺；备份密码派生和报表查询可能产生额外峰值。

单连接 SQLite 适合此个人版的简化目标；大流量、长历史报表和大量同时打开的看板需要另行压测。不能直接将 SQLite 官网中普通网站的请求量等同于访问统计服务的吞吐量。

## 验证

```sh
go test ./...
go test -race ./...
go vet ./...
cd frontend && bun run typecheck && bun run check:unused && bun run build
cd ..
./build.sh
```

Bun 只用于前端开发构建，不参与运行。测试仅创建新的临时 SQLite 文件，覆盖并发初始化、权限与 CSRF、会话主机绑定、埋点及统计口径、并发访问合并、筛选、分享权限、备份损坏与截断、完整恢复、两步验证恢复码、夏令时、原版图表数据、访客热力图、域名更改保留历史和内嵌路由资源。自动备份与云存储部分覆盖 AWS 文档中公布的 Signature V4 示例、云端地址策略与拒绝内网地址、凭据封存、计划调度与失败退避、本地保留、上传、从列表恢复，以及错误信息不泄漏密钥或地址；云端测试使用本地模拟的 S3 端点，没有连接真实的 R2 或 S3。Passkey 部分用软件认证器走完整的注册与登录流程，覆盖重放、过期、错误来源或域名、陌生凭据、用户句柄不符、计数器回退和备份恢复，但没有用真实的指纹、面容或安全密钥验证。站点检查部分覆盖地址与跳转规则、出站地址限制及放行网段、HTTP 判定、证书四种状态、RDAP 与 WHOIS 解析、调度与限流、设置变更时作废在途结果、保留期和级联删除；探测使用本地服务或替身，不访问真实网站。浏览器验收只使用 Google Chrome。

源码按 [MIT License](LICENSE) 分发，品牌和字体来自原版，字体许可随程序资源保留。
