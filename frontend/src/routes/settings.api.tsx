import { createFileRoute } from "@tanstack/react-router"
import { ApiKeyPanel } from "~/components/ApiKeyPanel"
import { SettingsTips } from "~/components/SettingsTips"

export const Route = createFileRoute("/settings/api")({
  component: () => <><ApiKeyPanel /><SettingsTips tips={[
    "API Key 只在创建时显示一次，请只保存在服务端的环境变量里，不要写进网页 JavaScript、公开仓库或网址参数。",
    "调用时在请求头加上 Authorization: Bearer <API Key>，请求 /api/v1/stats。完整的参数、返回字段和示例代码见官网 API 文档。",
    "不再使用时删除对应的 Key，调用会立即失效，不用改账号密码。删除站点时，它的 Key 也一起删除。",
  ]} /></>,
})
