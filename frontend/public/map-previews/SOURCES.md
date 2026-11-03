# Map style previews

Fixed preview images for the map style pickers. They contain no visitor data, no token or key, and no credentialed URL. At the owner's request (2026-11-03) the providers' logos and attribution strips were cropped out when the images were made; the maps on the statistics pages keep them.

## Mapbox (`standard.webp`, `streets-v12.webp`, `dark-v11.webp`)

Mapbox GL JS 3.33.0 screenshots captured in local Chrome on 2026-10-22 from the fixed application catalog (`mapbox://styles/mapbox/{styleId}`).

- Viewport: 720 × 440 pixels; camera longitude -122.44, latitude 37.77, zoom 11.3, bearing 0, pitch 0; Mercator.
- Standard: monochrome theme, day lighting, 3D objects and POI/transit/road labels disabled, matching the application configuration. Classic styles: unmodified default configuration.
- 2026-11-03: cropped to the top 648 × 396 pixels (x offset 36) to remove the logo and attribution, then scaled back to 720 × 440 (Lanczos) and saved as WebP quality 90.

To recapture larger, render the same camera at about 1080 × 700, crop away the bottom strip with the logo and attribution, scale to 720 × 440 and save as WebP quality 90. That needs an authorized public Mapbox token at runtime; never commit it.

## Google Maps (`google-standard.webp`, `google-roadmap.webp`, `google-dark.webp`)

Maps Static API images fetched once on 2026-11-03: center 37.77,-122.44, zoom 11, size 420x280, scale 2 (840 × 560 pixels), language en, with each style's rules from `googleMapLook` in `src/lib/google-map-styles.ts` converted to `style=` parameters (Streets is Google's unstyled road map). Standard in dark mode uses the Dark image, since `googleMapLook` gives both the same rules. Each was cropped to 838 × 512 at (1, 0) to remove the logo and "Map data" attribution, scaled to 720 × 440 and saved as WebP quality 90. Fetching again needs a key that allows the Static Maps API; never commit it or the request URL.
