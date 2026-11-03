import { type GoogleMapStyleId } from "~/lib/google-map-styles"

/**
 * A fixed preview of each Google Maps style: Static Maps captures at the Mapbox previews' place, cropped so Google's
 * logo and attribution do not show (public/map-previews/SOURCES.md). Standard follows light and dark mode, so it has
 * a light image and uses Dark's for dark mode, and CSS shows the one for the current mode.
 */
export function GoogleStylePreview({styleId, label}: {styleId: GoogleMapStyleId; label: string}) {
  const image = (file: string, mode?: string) => <img className="squircle map-style-preview" data-mode={mode} src={`/map-previews/google-${file}.webp`} alt={label} width="360" height="220" loading="lazy" decoding="async"/>
  return styleId === "standard" ? <>{image("standard", "light")}{image("dark", "dark")}</> : image(styleId)
}
