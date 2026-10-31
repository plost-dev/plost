import { UserAvatar } from "./UserAvatar"
import { ActionButton, ActionUpload } from "~/components/ActionButton"
import { AccentPicker } from "~/components/AccentPicker"
import { MapStylePicker } from "~/components/MapStylePicker"
import { useEffect, useState } from "react"
import { Field, FormActions, SettingsForm } from "~/components/SettingsForm"
import { Tile } from "~/components/ui"
import { updateProfileFn } from "~/lib/actions"
import { useT } from "~/lib/i18n"
import { showToast } from "~/lib/toast"

function resizeAvatar(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    if (!file.type.startsWith("image/")) {
      reject(new Error("请选择图片"))
      return
    }
    const img = new Image()
    const url = URL.createObjectURL(file)
    img.onload = () => {
      URL.revokeObjectURL(url)
      const canvas = document.createElement("canvas")
      canvas.width = 256
      canvas.height = 256
      const ctx = canvas.getContext("2d")
      if (!ctx) {
        reject(new Error("无法处理图片"))
        return
      }
      const min = Math.min(img.width, img.height)
      ctx.drawImage(img, (img.width - min) / 2, (img.height - min) / 2, min, min, 0, 0, 256, 256)
      const data = canvas.toDataURL("image/jpeg", 0.86)
      resolve(data.length > 200_000 ? canvas.toDataURL("image/jpeg", 0.7) : data)
    }
    img.onerror = () => {
      URL.revokeObjectURL(url)
      reject(new Error("无法读取图片"))
    }
    img.src = url
  })
}

export function AccountPreferences({ user, onSaved }: {
  user: { name: string, email: string, avatar?: string | null, gravatarUrl?: string }
  onSaved: () => void
}) {
  const { t, locale, setLocale } = useT()
  const [language,setLanguage] = useState(locale)
  useEffect(()=>setLanguage(locale),[locale])
  const [name, setName] = useState(user.name)
  const [avatar, setAvatar] = useState(user.avatar || "")
  const [avatarError, setAvatarError] = useState("")
  const [processing, setProcessing] = useState(false)
  useEffect(() => {setName(user.name);setAvatar(user.avatar || "")}, [user.name,user.avatar])
  return <Tile icon="user" title={t("个人资料")}>
    <SettingsForm className="preferences-form" onSubmit={async event=>{event.preventDefault();await updateProfileFn({data:{name,avatar:avatar || null,locale:language}});setLocale(language);onSaved();showToast("设置已保存")}}>
      <div className="settings-avatar-row">
        <UserAvatar avatar={avatar} gravatarUrl={user.gravatarUrl} name={name} className="squircle settings-avatar-preview" alt={t("profile.avatar")} />
        <div className="settings-avatar-controls flex min-w-0 flex-wrap gap-2">
          <ActionUpload  className="btn-secondary settings-avatar-upload" accept="image/*" disabled={processing} onChange={async event=>{
            const file=event.target.files?.[0];event.target.value="";if(!file)return
            setProcessing(true);setAvatarError("")
            try {setAvatar(await resizeAvatar(file))} catch(err){setAvatarError(err instanceof Error ? err.message : t("无法读取图片"))} finally {setProcessing(false)}
          }}>{t("profile.change_avatar")}</ActionUpload>
          {avatar ? <ActionButton  type="button" className="btn btn-secondary" onClick={()=>setAvatar("")}>{t("profile.remove_avatar")}</ActionButton> : null}
        </div>
      </div>
      {avatarError ? <p role="alert" className="mt-3 text-sm text-red-600">{t(avatarError)}</p> : null}
      <Field label={t("profile.nickname_label")}><input className="squircle input" required value={name} onChange={event=>setName(event.target.value)} /></Field>
      <Field label={t("邮箱")} hint={t("如需更改邮箱，请到「安全」页。")}><input className="squircle input bg-gray-50" value={user.email} disabled /></Field>
      <Field label={t("界面语言")}><select className="squircle input" value={language} onChange={event=>setLanguage(event.target.value === "en" ? "en" : "zh-CN")}><option value="zh-CN">简体中文</option><option value="en">English</option></select></Field>
      <FormActions><ActionButton className="btn btn-primary" type="submit" disabled={processing}>{t("保存设置")}</ActionButton></FormActions>
    </SettingsForm>
    <AccentPicker />
    <MapStylePicker />
  </Tile>
}
