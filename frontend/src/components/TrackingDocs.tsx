import { useEffect, useState } from "react"
import { UIIcon } from "./UIIcon"
import { TrackingSnippet } from "./TrackingSnippet"
import { useT } from "~/lib/i18n"

export function TrackingDocs({ domain }: { domain: string }) {
  const { t } = useT()
  const [openAll, setOpenAll] = useState<boolean | null>(null)

  const toggleAll = () => {
    setOpenAll(prev => (prev === true ? false : true))
  }

  const [baseUrl, setBaseUrl] = useState("https://stats.example.com")
  useEffect(() => { setBaseUrl(window.location.origin) }, [])

  return (
    <section id="api-docs" className="api-usage-docs" aria-label={t("埋点使用文档")}>
      <div className="api-docs-header">
        <div>
          <h3>
            <UIIcon name="code" />
            {t("开发者使用指南与接口文档")}
          </h3>
        </div>
        <button
          type="button"
          className="squircle api-docs-toggle-btn"
          onClick={toggleAll}
          title={openAll === true ? (t("收起所有折叠")) : (t("展开所有折叠"))}
        >
          
          <span>{openAll === true ? (t("全部折叠")) : (t("全部展开"))}</span>
        </button>
      </div>

      <div className="api-docs-sections">
        {/* Section 1: HTTP Event Ingestion API */}
        <details className="squircle api-doc-item" open={openAll ?? true}>
          <summary className="api-doc-summary">
            <div className="api-doc-summary-title">
              <UIIcon name="cloud" />
              <span>{t("1. HTTP 事件数据采集端点 (POST /api/event)")}</span>
            </div>
            <span className="squircle api-doc-badge">{t("采集接口")}</span>
          </summary>
          <div className="api-doc-content">
            <p>
              {t("支持从后端服务、移动 App、微信小程序、命令行脚本或无前端浏览器环境中直接向 Plost 上报页面访问与业务事件：")}
            </p>
            <div className="squircle api-endpoint-badge">
              <span className="squircle method-post">POST</span>
              <code>/api/event</code>
              <span className="endpoint-desc">{t("缓冲接收，按批写入")}</span>
            </div>

            <h4>{t("请求体参数格式 (JSON)")}</h4>
            <div className="squircle api-doc-table">
              <table>
                <thead>
                  <tr>
                    <th>{t("字段")}</th>
                    <th>{t("类型")}</th>
                    <th>{t("必须")}</th>
                    <th>{t("说明")}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td><code className="squircle">d</code> / <code className="squircle">domain</code></td>
                    <td>string</td>
                    <td><span className="squircle tag-required">{t("必须")}</span></td>
                    <td>{t("在 Plost 中添加的站点域名（如 demo.local.test）")}</td>
                  </tr>
                  <tr>
                    <td><code className="squircle">n</code> / <code className="squircle">name</code></td>
                    <td>string</td>
                    <td><span className="squircle tag-required">{t("必须")}</span></td>
                    <td>{t("事件名称（基础浏览填 'pageview'，自定义行为填如 'Signup'）")}</td>
                  </tr>
                  <tr>
                    <td><code className="squircle">u</code> / <code className="squircle">url</code></td>
                    <td>string</td>
                    <td><span className="squircle tag-required">{t("必须")}</span></td>
                    <td>{t("当前页面完整 URL（如 https://example.com/pricing）")}</td>
                  </tr>
                  <tr>
                    <td><code className="squircle">r</code> / <code className="squircle">referrer</code></td>
                    <td>string</td>
                    <td><span className="squircle tag-optional">{t("可选")}</span></td>
                    <td>{t("访问来源 Referrer（如 https://google.com）")}</td>
                  </tr>
                  <tr>
                    <td><code className="squircle">w</code> / <code className="squircle">width</code></td>
                    <td>number</td>
                    <td><span className="squircle tag-optional">{t("可选")}</span></td>
                    <td>{t("客户端屏幕宽度像素（如 1440，用于识别移动/桌面设备）")}</td>
                  </tr>
                  <tr>
                    <td><code className="squircle">p</code> / <code className="squircle">props</code></td>
                    <td>object</td>
                    <td><span className="squircle tag-optional">{t("可选")}</span></td>
                    <td>{t("自定义维度键值对（如 {\"plan\": \"pro\", \"source\": \"hero\"}）")}</td>
                  </tr>
                </tbody>
              </table>
            </div>

            <h4>{t("cURL 上报调用示例")}</h4>
            <TrackingSnippet
              code={`curl -X POST ${baseUrl}/api/event \\
  -H "Content-Type: application/json" \\
  -H "User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36" \\
  -d '{
    "d": ${JSON.stringify(domain)},
    "n": "pageview",
    "u": ${JSON.stringify(`https://${domain}/docs`)},
    "r": "https://google.com",
    "w": 1440,
    "p": { "plan": "pro" }
  }'`}
              language="cURL"
            />
            <p className="api-note">
              {t("有效事件在批次提交到 SQLite 后返回 HTTP 202 Accepted。被过滤的机器人、域名不匹配或尚未添加的站点也会返回 202，但不会计入统计。")}
            </p>
          </div>
        </details>

        {/* Section 2: Web Tracker Integration */}
        <details className="squircle api-doc-item" open={openAll ?? false}>
          <summary className="api-doc-summary">
            <div className="api-doc-summary-title">
              <UIIcon name="globe" />
              <span>{t("2. 网页埋点脚本与 JavaScript API")}</span>
            </div>
            <span className="squircle api-doc-badge">{t("网页集成")}</span>
          </summary>
          <div className="api-doc-content">
            <p>
              {t("将统计脚本放入网站的 <head> 标签内，只需添加一次。defer 让脚本随页面并行下载，在 HTML 解析完成后执行；自动记录首次浏览，以及单页应用（SPA）的路径或查询参数变化：")}
            </p>
            <TrackingSnippet
              code={`<script defer data-domain="${domain}" src="${baseUrl}/script.js"></script>`}
              language="HTML"
            />
            <p className="api-note">
              {t("脚本最多缓存 5 分钟。使用 localStorage 持久保存随机匿名访客 ID，不使用 Cookie；存储不可用时回退到原每日匿名标识。同一地址的历史状态更新和仅锚点变化不会重复计数；SPA 路由切换保留本次文档的入站来源，用于来源统计。预渲染页面在实际打开后才计数，浏览器从往返缓存恢复页面会记录一次新浏览。访问时长由已记录的页面浏览估算，最后一次浏览之后的停留时间无法精确测量。")}
            </p>
            <p className="api-note">
              {t("上报使用 keepalive 请求，不支持 fetch 时使用 Beacon。只有服务端明确表示缓冲区已满、尚未接收的事件才最多重试两次；网络结果不确定时不会重复补发，以免重复计数。广告拦截、隐私设置、断网、浏览器关闭和服务中断仍可能造成缺失，有效事件在批次提交后才返回 HTTP 202；被过滤的请求不计入统计。")}
            </p>

            <h4>{t("在 JavaScript 中触发自定义事件")}</h4>
            <p>
              {t("在页面任意交互事件中调用 window.plost.track()：")}
            </p>
            <TrackingSnippet
              code={`// Track a custom event
if (window.plost) {
  window.plost.track("Button Click");
}`}
              language="JavaScript"
            />
          </div>
        </details>


      </div>
    </section>
  )
}
