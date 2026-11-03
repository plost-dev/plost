import { useState } from "react"
import { useT } from "~/lib/i18n"
import { clientIPCheckFn, type ClientIPCheck, type ClientIPWarning } from "~/lib/client-ip-actions"
import { ActionButton } from "./ActionButton"
import { Tile } from "./ui"
import "~/styles/backup-settings.css"

const KIND_LABEL: Record<string, string> = {public: "公网地址", cloudflare: "Cloudflare 节点", private: "内网地址", loopback: "本机地址", invalid: "无法识别"}
const VIA_LABEL: Record<string, string> = {
  direct: "直接连接的地址（没有受信任的代理）",
  header: "受信任代理给的地址头",
  forwarded: "X-Forwarded-For 链（从右往左跳过受信任的代理）",
  "x-real-ip": "受信任代理给的 X-Real-IP",
  proxy: "受信任的代理本身（它没有转来访客地址）",
}
const WARNING_TEXT: Record<ClientIPWarning["code"], string> = {
  untrusted_proxy: "{0} 的统计请求带着转发头，但发来请求的代理不受信任，所以访客都被记成了这个代理。",
  proxy_addresses: "{0} 的访客地址是本机或内网地址，前面可能还有一层代理（例如 Docker）没有被信任。",
  cdn_addresses: "{0} 的访客地址是 Cloudflare 节点，Cloudflare 还没有被信任。",
  shared_network: "{0} 的访客来自同一个网段，可能是 CDN 节点被当成了访客。",
}
const SETUPS: Array<[string, string]> = [
  ["域名直接解析到服务器，没有代理", "不用设置"],
  ["本机的 Caddy、Nginx 反向代理", "-trusted-proxies loopback"],
  ["反向代理在另一个 Docker 容器或内网机器", "-trusted-proxies private"],
  ["Cloudflare 橙云，回源到本机反代", "-trusted-proxies loopback,cloudflare"],
  ["Cloudflare 橙云，直接回源到 Plost", "-trusted-proxies cloudflare"],
  ["阿里云、腾讯云等其他 CDN", "-trusted-proxies loopback,<CDN 回源网段> 或 @文件"],
]

const percent = (share: number) => `${Math.round(share * 100)}%`

export function ClientIPPanel({initial}: {initial: ClientIPCheck}) {
  const {t} = useT()
  const [check, setCheck] = useState(initial)
  const {config, request, recent} = check
  const headers = Object.entries(request.headers)
  const kinds = Object.entries(recent.kinds).sort((a, b) => b[1] - a[1])
  return <>
    <Tile icon="globe" title="访客 IP 识别" subtitle="检查统计记下的是不是访客自己的地址，而不是本机、代理或 CDN 节点的地址。">
      <h3 className="text-sm font-semibold text-gray-800">{t("最近的统计请求")}</h3>
      {recent.total < 30
        ? <p className="mt-2 text-sm leading-6 text-gray-500">{t("最近只收到 {0} 次统计请求，还不足以判断，稍后再来看。", {0: recent.total})}</p>
        : recent.warnings.length === 0
          ? <p className="mt-2 text-sm leading-6 text-gray-700">{t("最近 {0} 次统计请求没有发现问题。", {0: recent.total})}</p>
          : <ul className="mt-3 grid gap-3">{recent.warnings.map(w => <li key={w.code} className="squircle rounded-md bg-amber-50 px-4 py-3 text-sm leading-6 text-amber-900">
              {t(WARNING_TEXT[w.code], {0: percent(w.share)})}
              <div className="mt-1 text-amber-800">{w.suggest
                ? t("在启动参数的 -trusted-proxies 里加上 {0}，然后重启 Plost。", {0: w.suggest})
                : w.code === "shared_network"
                  ? t("确认 CDN 的回源网段已经加入 -trusted-proxies，或者用 -client-ip-header 指定 CDN 给的真实 IP 头。")
                  : t("把这个 CDN 的回源网段加入 -trusted-proxies，然后重启 Plost。")}</div>
            </li>)}</ul>}
      {recent.total > 0 ? <div className="mt-4"><dl className="squircle backup-status-grid">
        <div><dt>{t("统计请求")}</dt><dd>{recent.total}</dd></div>
        <div><dt>{t("访客地址类型")}</dt><dd>{kinds.map(([kind, n]) => `${t(KIND_LABEL[kind] || kind)} ${percent(n / recent.total)}`).join("，")}</dd></div>
        <div><dt>{t("转发头被忽略")}</dt><dd>{percent(recent.ignored / recent.total)}</dd></div>
        <div><dt>{t("最集中的网段")}</dt><dd>{t("{0} 的访客，共 {1} 个网段", {0: percent(recent.topShare), 1: recent.networks})}</dd></div>
      </dl></div> : null}
      <p className="mt-3 text-xs leading-6 text-gray-500">{t("只在内存里统计最近 1000 次请求的地址类型和网段哈希，不保存访客地址；重启后重新计算。")}</p>

      <div className="backup-credentials">
        <h3>{t("这次访问")}</h3>
        <p>{t("你打开这个页面的请求，按现在的设置是这样识别的。")}</p>
        <dl className="squircle backup-status-grid">
          <div><dt>{t("直接连接的地址")}</dt><dd>{request.peer || "—"}（{t(request.peerTrusted ? "受信任的代理" : "不受信任")}）</dd></div>
          <div><dt>{t("认定的访客地址")}</dt><dd>{request.address || "—"}（{t(KIND_LABEL[request.kind] || request.kind)}）</dd></div>
          <div><dt>{t("识别方式")}</dt><dd>{t(VIA_LABEL[request.via] || request.via)}</dd></div>
          <div><dt>{t("国家")}</dt><dd>{request.country || t("未知")}</dd></div>
          <div className="backup-status-id"><dt>{t("收到的转发头")}</dt><dd>{headers.length ? headers.map(([name, value]) => <div key={name}><code>{name}: {value}</code></div>) : t("没有")}</dd></div>
        </dl>
        {!request.known ? <p className="mt-3 text-sm leading-6 text-amber-800">{t("这个地址不是访客自己的：请求来自本机或带着转发头，但代理不受信任。这样的访客按浏览器标识计数。")}</p> : null}
      </div>

      <div className="backup-credentials">
        <h3>{t("当前设置")}</h3>
        <p>{t("这些是启动参数，只能在服务器上修改，改完重启 Plost 生效。")}</p>
        <dl className="squircle backup-status-grid">
          <div><dt>-trusted-proxies</dt><dd>{config.trusted.length ? config.trusted.join(", ") : t("未设置，不信任任何代理")}</dd></div>
          <div><dt>-client-ip-header</dt><dd>{config.header || t("未设置，读取 X-Forwarded-For")}</dd></div>
          <div><dt>-geoip-db</dt><dd>{t(config.geoip ? "已配置" : "未配置，只能靠 CDN 的国家头")}</dd></div>
        </dl>
      </div>

      <div className="backup-credentials">
        <h3>{t("怎么设置")}</h3>
        <dl className="squircle backup-status-grid">
          {SETUPS.map(([setup, flag]) => <div key={setup} className="backup-status-id"><dt>{t(setup)}</dt><dd><code>{t(flag)}</code></dd></div>)}
        </dl>
        <p className="mt-3 text-xs leading-6 text-gray-500">{t("CDN 的回源网段要在 CDN 控制台或接口里查。前面有本机反代时，它要把收到的 X-Forwarded-For 追加或覆盖后再转给 Plost。直接访问源站伪造的 CF-Connecting-IP、CF-IPCountry 等头不会被采用。")}</p>
      </div>
      <div className="mt-6"><ActionButton type="button" className="btn btn-secondary" onClick={async () => setCheck(await clientIPCheckFn())}>{t("重新检查")}</ActionButton></div>
    </Tile>
  </>
}
