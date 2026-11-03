import { useState } from "react"
import { useT } from "~/lib/i18n"
import { showToast } from "~/lib/toast"
import { clientIPCheckFn, type ClientIPCheck, type ClientIPWarning } from "~/lib/client-ip-actions"
import { ActionButton } from "./ActionButton"
import { UIIcon } from "./UIIcon"
import { Tile } from "./ui"
import { SettingsTips } from "./SettingsTips"
import "~/styles/client-ip.css"

const KINDS: Array<{ id: string; label: string }> = [
  { id: "public", label: "公网地址" },
  { id: "cloudflare", label: "Cloudflare 节点" },
  { id: "private", label: "内网地址" },
  { id: "loopback", label: "本机地址" },
  { id: "invalid", label: "无法识别" },
]
const KIND_LABEL = Object.fromEntries(KINDS.map(k => [k.id, k.label]))
const VIAS: Array<{ id: string; label: string; hint: string }> = [
  { id: "forwarded", label: "转发链", hint: "X-Forwarded-For 从右往左，跳过受信任的代理" },
  { id: "header", label: "指定的地址头", hint: "-client-ip-header 指定的头" },
  { id: "x-real-ip", label: "X-Real-IP", hint: "受信任代理给的 X-Real-IP" },
  { id: "direct", label: "直接连接", hint: "没有受信任的代理，连接方就是访客" },
  { id: "proxy", label: "代理本身", hint: "受信任的代理没有转来访客地址" },
]
const WARNING_TEXT: Record<ClientIPWarning["code"], { title: string; detail: string }> = {
  untrusted_proxy: { title: "前面的代理没有被信任", detail: "{0} 的统计请求带着转发头，但发来请求的代理不受信任，所以访客都被记成了这个代理。" },
  proxy_addresses: { title: "访客地址是本机或内网地址", detail: "{0} 的访客地址是本机或内网地址，前面可能还有一层代理（例如 Docker）没有被信任。" },
  cdn_addresses: { title: "访客地址是 CDN 节点", detail: "{0} 的访客地址是 Cloudflare 节点，Cloudflare 还没有被信任。" },
  shared_network: { title: "大部分访客来自同一个网段", detail: "{0} 的访客来自同一个网段，可能是 CDN 节点被当成了访客。" },
}
const SETUPS: Array<[string, string]> = [
  ["域名直接解析到服务器，没有代理", "不用设置"],
  ["本机的 Caddy、Nginx 反向代理", "-trusted-proxies loopback"],
  ["反向代理在另一个 Docker 容器或内网机器", "-trusted-proxies private"],
  ["Cloudflare 橙云，回源到本机反代", "-trusted-proxies loopback,cloudflare"],
  ["Cloudflare 橙云，直接回源到 Plost", "-trusted-proxies cloudflare"],
  ["阿里云、腾讯云等其他 CDN", "-trusted-proxies loopback,<CDN 回源网段> 或 @文件"],
]
const MIN_SAMPLES = 30
const percent = (share: number) => `${Math.round(share * 100)}%`

async function copy(text: string) {
  try { await navigator.clipboard.writeText(text); showToast("已复制") } catch { showToast("复制失败，请手动选择文字复制。", "error") }
}

export function ClientIPPanel({ initial }: { initial: ClientIPCheck }) {
  const { t } = useT()
  const [check, setCheck] = useState(initial)
  const { config, request, recent, suggested } = check
  const enough = recent.total >= MIN_SAMPLES
  const verdict = !enough ? "idle" : recent.warnings.length ? "warn" : "ok"
  const share = (n: number | undefined) => recent.total ? (n || 0) / recent.total : 0
  const kinds = KINDS.filter(k => recent.kinds[k.id])
  const headers = Object.entries(request.headers)

  return <><Tile icon="shield" title="访客 IP 识别"
    actions={<ActionButton type="button" className="btn btn-secondary" onClick={async () => setCheck(await clientIPCheckFn())}>{t("重新检查")}</ActionButton>}>

    <div className={`squircle ipcheck-verdict is-${verdict}`} role="status">
      <span className="squircle ipcheck-verdict-icon"><UIIcon name={verdict === "ok" ? "check" : verdict === "warn" ? "warn" : "clock"} /></span>
      <div>
        <strong>{t(verdict === "ok" ? "识别正常" : verdict === "warn" ? "需要调整启动参数" : "数据还不够")}</strong>
        <p>{verdict === "ok" ? t("最近 {0} 次统计请求认出的都是访客自己的地址。", { 0: recent.total })
          : verdict === "warn" ? t("最近 {0} 次统计请求里发现 {1} 个问题，按下面的建议修改后重启 Plost。", { 0: recent.total, 1: recent.warnings.length })
          : t("最近只收到 {0} 次统计请求，至少需要 {1} 次才能判断，稍后再来看。", { 0: recent.total, 1: MIN_SAMPLES })}</p>
      </div>
    </div>

    <dl className="ipcheck-stats">
      <div className="squircle"><dt>{t("统计请求")}</dt><dd>{recent.total}</dd><span>{t("最近 1000 次以内")}</span></div>
      <div className="squircle"><dt>{t("公网地址")}</dt><dd>{recent.total ? percent(share(recent.kinds.public)) : "—"}</dd><span>{t("越接近 100% 越好")}</span></div>
      <div className="squircle"><dt>{t("转发头被忽略")}</dt><dd>{recent.total ? percent(share(recent.ignored)) : "—"}</dd><span>{t("应为 0%")}</span></div>
      <div className="squircle"><dt>{t("不同网段")}</dt><dd>{recent.networks}</dd><span>{recent.total ? t("最多的占 {0}", { 0: percent(recent.topShare) }) : t("暂无")}</span></div>
    </dl>

    {recent.total ? <div className="ipcheck-charts">
      <section>
        <h3>{t("访客地址类型")}</h3>
        <div className="squircle ipcheck-bar" role="img" aria-label={kinds.map(k => `${t(k.label)} ${percent(share(recent.kinds[k.id]))}`).join("，")}>
          {kinds.map(k => <span key={k.id} className={`is-${k.id}`} style={{ width: `${share(recent.kinds[k.id]) * 100}%` }} />)}
        </div>
        <ul className="ipcheck-legend">{kinds.map(k => <li key={k.id}><i className={`is-${k.id}`} />{t(k.label)}<strong>{percent(share(recent.kinds[k.id]))}</strong><span>{recent.kinds[k.id]}</span></li>)}</ul>
      </section>
      <section>
        <h3>{t("识别方式")}</h3>
        <ul className="ipcheck-rows">{VIAS.filter(v => recent.via[v.id]).map(v => <li key={v.id} title={t(v.hint)}>
          <span>{t(v.label)}</span>
          <span className="squircle ipcheck-row-track"><span style={{ width: `${share(recent.via[v.id]) * 100}%` }} /></span>
          <strong>{percent(share(recent.via[v.id]))}</strong>
        </li>)}</ul>
      </section>
    </div> : null}

    {recent.warnings.length ? <section className="ipcheck-section">
      <h3>{t("发现的问题")}</h3>
      <ul className="ipcheck-warnings">{recent.warnings.map(w => <li key={w.code} className="squircle">
        <UIIcon name="warn" />
        <div><strong>{t(WARNING_TEXT[w.code].title)}</strong><p>{t(WARNING_TEXT[w.code].detail, { 0: percent(w.share) })}</p>
          <p className="ipcheck-advice">{w.suggest ? t("在 -trusted-proxies 里加上 {0}。", { 0: w.suggest })
            : w.code === "shared_network" ? t("把 CDN 的回源网段加入 -trusted-proxies，或者用 -client-ip-header 指定 CDN 给的真实 IP 头。")
            : t("把这个 CDN 的回源网段加入 -trusted-proxies。")}</p></div>
      </li>)}</ul>
    </section> : null}

    {suggested ? <section className="ipcheck-section">
      <h3>{t("推荐的启动参数")}</h3>
      <div className="squircle ipcheck-command"><code>{suggested}</code><button type="button" className="squircle btn btn-secondary" onClick={() => void copy(suggested)}><UIIcon name="copy" />{t("复制")}</button></div>
      <p className="ipcheck-note">{t("替换服务器上 Plost 启动命令（例如 systemd 的 ExecStart）里原来的 -trusted-proxies 或 -trust-proxy，然后重启 Plost。")}</p>
    </section> : null}

    <section className="ipcheck-section">
      <h3>{t("这次访问的链路")}</h3>
      <p className="ipcheck-note">{t("你打开这个页面的请求从左边的访客出发，经过各层代理到达 Plost。")}</p>
      <ol className="ipcheck-hops">
        {request.hops.map((hop, i) => <li key={i} className={`squircle${hop.visitor ? " is-visitor" : ""}${hop.trusted && !hop.visitor ? " is-trusted" : ""}`}>
          <span className="ipcheck-hop-role">{t(hop.visitor ? "认定的访客" : hop.trusted ? "受信任的代理" : "不受信任")}</span>
          <code>{hop.address}</code>
          <span className="ipcheck-hop-kind">{t(KIND_LABEL[hop.kind] || hop.kind)}</span>
        </li>)}
        <li className="squircle is-plost"><span className="ipcheck-hop-role">Plost</span><code>{request.country || t("国家未知")}</code><span className="ipcheck-hop-kind">{t(VIAS.find(v => v.id === request.via)?.label || request.via)}</span></li>
      </ol>
      {!request.known ? <p className="ipcheck-note is-warn">{t("这个地址不是访客自己的：请求来自本机或带着转发头，但代理不受信任。这样的访客按浏览器标识计数。")}</p> : null}
      {headers.length ? <details className="ipcheck-headers"><summary>{t("收到的转发头（{0}）", { 0: headers.length })}</summary>
        <dl>{headers.map(([name, value]) => <div key={name}><dt>{name}</dt><dd><code>{value}</code></dd></div>)}</dl>
      </details> : null}
    </section>

    <section className="ipcheck-section">
      <h3>{t("当前设置")}</h3>
      <dl className="ipcheck-config">
        <div><dt>-trusted-proxies</dt><dd>{config.trusted.length ? config.trusted.map(name => <code key={name} className="squircle">{name}</code>) : <span>{t("未设置，不信任任何代理")}</span>}</dd></div>
        <div><dt>-client-ip-header</dt><dd>{config.header ? <code className="squircle">{config.header}</code> : <span>{t("未设置，读取 X-Forwarded-For")}</span>}</dd></div>
        <div><dt>-geoip-db</dt><dd><span>{t(config.geoip ? "已配置" : "未配置，只能靠 CDN 的国家头")}</span></dd></div>
      </dl>
    </section>

    <section className="ipcheck-section">
      <h3>{t("怎么设置")}</h3>
      <table className="ipcheck-table"><thead><tr><th>{t("部署方式")}</th><th>{t("启动参数")}</th></tr></thead>
        <tbody>{SETUPS.map(([setup, flag]) => <tr key={setup}><td>{t(setup)}</td><td><code>{t(flag)}</code></td></tr>)}</tbody></table>
    </section>
  </Tile>
  <SettingsTips tips={[
    "这页检查统计记下的是不是访客自己的地址，而不是本机、代理或 CDN 节点的地址。",
    "-trusted-proxies 等是启动参数，只能在服务器上修改，改完重启 Plost 生效。",
    "CDN 的回源网段要在 CDN 控制台或接口里查。前面有本机反代时，它要把收到的 X-Forwarded-For 追加或覆盖后再转给 Plost。直接访问源站伪造的 CF-Connecting-IP、CF-IPCountry 等头不会被采用。",
    "只在内存里统计最近 1000 次请求的地址类型和网段哈希，不保存访客地址；重启后重新计算。",
  ]} /></>
}
