/**
 * What someone pasted as a site's address, cut down to its host: the scheme, a user name, www., the port, path, query
 * and fragment go, so "https://www.example.com/blog?x=1" becomes "example.com". The server does the same.
 */
export function cleanDomainInput(value: string) {
  let host = value.trim().toLowerCase()
  host = host.replace(/^[a-z][a-z0-9+.-]*:\/\//, "")
  host = host.replace(/^[^/?#]*@/, "")
  host = host.split(/[/?#]/)[0]
  host = host.replace(/:\d*$/, "").replace(/\.$/, "")
  return host.replace(/^www\./, "")
}
