# Province and state boundaries

`world-regions.bin` holds the first-level administrative divisions of every country (4,596 provinces, states,
departments and counties) for the built-in visitor map. It is derived from Natural Earth's 1:10m
"Admin 1 – States, Provinces" (version 5.1.1), which is in the public domain:

- https://naciscdn.org/naturalearth/10m/cultural/ne_10m_admin_1_states_provinces.zip
- https://www.naturalearthdata.com/about/terms-of-use/

`scripts/build-region-map.ts` builds it with mapshaper 0.6.102: shapes simplified to 15% (weighted, small islands
kept), quantized to 100,000 steps, and written in the LSM1 layout that `src/lib/world-map-format.ts` reads. Each
area keeps its country, its ISO 3166-2 code and its English and Chinese names. The script also files France's
overseas regions, Svalbard, Bouvet Island, Christmas Island and the Cocos Islands under their own country codes,
as GeoIP databases do, and gives Poland's and Czechia's regions their current numeric ISO codes.

```sh
bun scripts/build-region-map.ts ne_10m_admin_1_states_provinces.shp node_modules/.bin/mapshaper
```

Countries are not stored separately: the map merges each country's provinces, so country and province borders
come from the same lines. `world-50m.bin` remains the source of the site overview's globe.
