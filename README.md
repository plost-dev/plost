<p align="center">
  <a href="https://plo.st"><img src="docs/assets/plost-logo.svg" width="240" alt="Plost"></a>
</p>

<p align="center"><b>好看、轻巧、只属于你的网站统计。</b></p>

<p align="center">
  <a href="https://go.dev/"><img alt="Go 1.27" src="https://img.shields.io/badge/Go-1.27-18181b?style=flat-square&logo=go&logoColor=white&labelColor=27272a"></a>
  <a href="https://www.sqlite.org/"><img alt="SQLite" src="https://img.shields.io/badge/SQLite-Embedded-18181b?style=flat-square&logo=sqlite&logoColor=white&labelColor=27272a"></a>
  <a href="LICENSE"><img alt="MIT License" src="https://img.shields.io/badge/License-MIT-18181b?style=flat-square&labelColor=27272a"></a>
</p>

<p align="center">
  <a href="https://plo.st">官网</a> ·
  <a href="docs/guide.md">使用指南</a>
</p>

---

谁来过、从哪来、看了什么——打开 Plost，一眼就清楚。

不用再把访客数据交给第三方，也不用为一个统计面板养一整套数据库。Plost 是一个文件，下载、运行，五分钟后你的网站就有了一块干净漂亮的数据看板。

## 一个文件，就是全部

没有 Docker，没有 Node，没有 PostgreSQL。界面、脚本、数据库全都装在一个约 24 MB 的程序里，小内存 VPS 也跑得轻松。

```sh
./plost -listen 127.0.0.1:3101 -data ./data
```

## 该看的，都在这里

- **核心指标**：访客、访问、浏览量、跳出率、停留时长，外加实时在线人数
- **流量来源**：来源网站、UTM 活动、热门页面
- **访客画像**：国家地图、浏览器、系统、设备、语言
- **趋势与时段**：可缩放的趋势曲线、一周访问热力图、任意维度筛选
- **事件**：外链点击、文件下载、自定义事件，接入即用
- **站点健康**：网站是否在线、SSL 证书和域名还剩几天到期

## 尊重每一位访客

不保存原始 IP，不跨月追踪同一个人，不把数据发给任何第三方。所有统计都在你自己的服务器上完成。

## 安心交给它

- **安全登录**：密码、两步验证、Passkey 指纹 / 面容登录
- **加密备份**：一键下载，定时自动备份，可同步到 Cloudflare R2 或 S3
- **一键恢复**：后台上传即可还原，恢复前自动留存当前数据
- **放心分享**：生成只读链接给团队或客户，随时撤销；也可开放公开统计页

## 用过 Umami？

统计口径与 Umami 一致，数字可以直接对照。自带搬迁工具，历史数据逐日对账后完整迁入，一条不多、一条不少。

## 细节也讲究

中文与英文、深色与浅色、四种强调色，手机上同样好用。

## 三步开始

1. 获取对应平台的程序（macOS / Linux，x86 与 ARM），构建方法见[使用指南](docs/guide.md)
2. 运行它，打开浏览器创建管理员账号
3. 把一行脚本放进你网站的 `<head>`：

```html
<script defer data-domain="example.com" src="https://stats.example.com/script.js"></script>
```

部署、反向代理、备份与搬迁的完整说明见 [使用指南](docs/guide.md)。

---

<p align="center"><a href="LICENSE">MIT</a> 开源 · 自由使用、修改与分发</p>
