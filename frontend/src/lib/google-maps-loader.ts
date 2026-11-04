// The Maps JavaScript API has no bundled types here; only the few calls the maps make are used.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type Maps = any
declare global { interface Window { google?: { maps: Maps }; __plostGoogleMapsReady?: () => void; gm_authFailure?: () => void } }

let loading: Promise<Maps> | null = null
/**
 * Loads the Maps JavaScript API once per page. A rejected key (gm_authFailure) or a failed script rejects. It lives on
 * its own so the site overview can use it without pulling in the statistics page's Google map.
 */
export function loadGoogleMaps(key: string, language: string): Promise<Maps> {
  if (window.google?.maps?.Map) return Promise.resolve(window.google.maps)
  if (loading) return loading
  loading = new Promise((resolve, reject) => {
    window.__plostGoogleMapsReady = () => resolve(window.google!.maps)
    const script = document.createElement("script")
    script.src = `https://maps.googleapis.com/maps/api/js?key=${encodeURIComponent(key)}&v=weekly&loading=async&language=${language}&callback=__plostGoogleMapsReady`
    script.async = true
    script.onerror = () => { loading = null; script.remove(); reject(new Error("Google Maps could not be loaded")) }
    document.head.append(script)
  })
  return loading
}
