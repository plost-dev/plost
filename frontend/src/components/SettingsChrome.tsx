import { UIIcon } from "~/components/UIIcon"
import { SiteFavicon } from "~/components/SiteFavicon"
import { useT } from "~/lib/i18n"
import type { ReactNode } from "react"

/** Header contract for management pages; analytics keeps its own toolbar. */
export function ManagementHeader({title,description,icon,children}: {title:ReactNode;description?:string;icon?:string;children?:ReactNode}) {
  return <header className="squircle management-heading"><div><h1 className="management-heading-title">{icon ? <UIIcon name={icon}/> : null}{title}</h1>{description ? <p>{description}</p> : null}</div>{children ? <div className="management-heading-actions">{children}</div> : null}</header>
}

export function SettingsHeader({
  title,
  domain,
  name,
  back,
  iconVersion = 0,
}: {
  title: string
  domain?: string
  name?: string
  back: ReactNode
  /** When the site's uploaded icon last changed, so the icon beside the address is current. */
  iconVersion?: number
}) {
  const { t } = useT()
  const display = domain ? (name && name.toLowerCase() !== domain.toLowerCase() ? `${name} (${domain})` : domain) : ""
  return (
    <ManagementHeader icon="settings" title={domain ? <span className="site-settings-title"><span>{t(title)}</span><span className="site-settings-domain"><SiteFavicon domain={domain} version={iconVersion} className="site-settings-favicon"/><span>{display}</span></span></span> : t(title || "")}>{back}</ManagementHeader>
  )
}
