"use client";

import { useEffect, useRef, useState } from "react";
import type { GeoJSONSource, Map as MapLibreMap, MapLayerMouseEvent } from "maplibre-gl";
import "maplibre-gl/dist/maplibre-gl.css";

import { COUNTRY_CENTROIDS } from "@/lib/world/country-centroids";
import type { WorldCountry } from "@/lib/world/world-api";
import { useLanguage } from "@/lib/language-context";

/**
 * Open, keyless vector style by default; override with NEXT_PUBLIC_MAP_STYLE_URL
 * (any MapLibre style URL, e.g. a self-hosted or commercial one).
 */
const MAP_STYLE_URL = process.env.NEXT_PUBLIC_MAP_STYLE_URL || "https://tiles.openfreemap.org/styles/positron";

const SOURCE = "world-countries";
const FONT = ["Noto Sans Regular"];

function cssColor(name: string, fallback: string): string {
  if (typeof window === "undefined") return fallback;
  const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  return v || fallback;
}

function reducedMotion(): boolean {
  return typeof window !== "undefined" && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}

function toGeoJSON(countries: WorldCountry[]): GeoJSON.FeatureCollection<GeoJSON.Point> {
  return {
    type: "FeatureCollection",
    features: countries
      .filter((c) => COUNTRY_CENTROIDS[c.countryCode])
      .map((c) => ({
        type: "Feature",
        properties: { code: c.countryCode, people: c.people },
        geometry: { type: "Point", coordinates: COUNTRY_CENTROIDS[c.countryCode] },
      })),
  };
}

/**
 * Country-level map of discoverable people. Markers are aggregated counts at
 * a country's geographic centre — never an individual's location. Nearby
 * countries cluster (summing real counts); clicking a cluster zooms in and
 * clicking a country selects it.
 */
export function WorldMap({
  countries,
  selected,
  onSelect,
}: {
  countries: WorldCountry[];
  selected: string | null;
  onSelect: (code: string) => void;
}) {
  const { t } = useLanguage();
  const containerRef = useRef<HTMLDivElement | null>(null);
  const mapRef = useRef<MapLibreMap | null>(null);
  const onSelectRef = useRef(onSelect);
  const [state, setState] = useState<"loading" | "ready" | "error">("loading");

  useEffect(() => {
    onSelectRef.current = onSelect;
  }, [onSelect]);

  // Create the map once.
  useEffect(() => {
    let cancelled = false;
    let map: MapLibreMap | null = null;
    let resizeObserver: ResizeObserver | null = null;
    void import("maplibre-gl")
      .then(({ default: maplibregl }) => {
        if (cancelled || !containerRef.current) return;
        const primary = cssColor("--color-primary", "#2563eb");
        map = new maplibregl.Map({
          container: containerRef.current,
          style: MAP_STYLE_URL,
          center: [20, 25],
          zoom: 1.1,
          minZoom: 0.6,
          maxZoom: 6,
          // Full attribution (OpenFreeMap / OpenMapTiles / OpenStreetMap) stays visible.
          attributionControl: { compact: false },
          dragRotate: false,
          pitchWithRotate: false,
          renderWorldCopies: false,
        });
        map.touchZoomRotate.disableRotation();
        map.addControl(new maplibregl.NavigationControl({ showCompass: false }), "top-right");
        mapRef.current = map;
        // Resize with the container (panel/rail/layout changes), not only the window.
        if (typeof ResizeObserver !== "undefined") {
          resizeObserver = new ResizeObserver(() => map?.resize());
          resizeObserver.observe(containerRef.current);
        }

        let loaded = false;
        map.on("error", () => {
          // Before the style loads, any error means the map cannot render.
          if (!loaded && !cancelled) setState("error");
        });
        map.on("load", () => {
          if (cancelled || !map) return;
          loaded = true;
          map.addSource(SOURCE, {
            type: "geojson",
            data: { type: "FeatureCollection", features: [] },
            cluster: true,
            clusterRadius: 38,
            clusterMaxZoom: 4,
            clusterProperties: { people: ["+", ["get", "people"]] },
          });
          const radius: maplibregl.ExpressionSpecification = ["interpolate", ["linear"], ["get", "people"], 1, 13, 10, 18, 100, 26, 1000, 34];
          map.addLayer({
            id: "clusters",
            type: "circle",
            source: SOURCE,
            filter: ["has", "point_count"],
            paint: { "circle-color": primary, "circle-opacity": 0.85, "circle-radius": radius, "circle-stroke-width": 3, "circle-stroke-color": "#ffffff" },
          });
          map.addLayer({
            id: "countries",
            type: "circle",
            source: SOURCE,
            filter: ["!", ["has", "point_count"]],
            paint: {
              "circle-color": ["case", ["==", ["get", "code"], ""], "#0f172a", primary],
              "circle-opacity": 0.9,
              "circle-radius": radius,
              "circle-stroke-width": 3,
              "circle-stroke-color": "#ffffff",
            },
          });
          map.addLayer({
            id: "counts",
            type: "symbol",
            source: SOURCE,
            layout: {
              "text-field": ["to-string", ["get", "people"]],
              "text-font": FONT,
              "text-size": 12,
              "text-allow-overlap": true,
            },
            paint: { "text-color": "#ffffff" },
          });

          map.on("click", "clusters", (e: MapLayerMouseEvent) => {
            const feature = e.features?.[0];
            if (!feature || !map) return;
            const clusterId = feature.properties?.cluster_id as number;
            const source = map.getSource(SOURCE) as GeoJSONSource;
            void source.getClusterExpansionZoom(clusterId).then((zoom) => {
              map?.easeTo({
                center: (feature.geometry as GeoJSON.Point).coordinates as [number, number],
                zoom,
                duration: reducedMotion() ? 0 : 450,
              });
            });
          });
          map.on("click", "countries", (e: MapLayerMouseEvent) => {
            const code = e.features?.[0]?.properties?.code;
            if (typeof code === "string") onSelectRef.current(code);
          });
          for (const layer of ["clusters", "countries"]) {
            map.on("mouseenter", layer, () => {
              if (map) map.getCanvas().style.cursor = "pointer";
            });
            map.on("mouseleave", layer, () => {
              if (map) map.getCanvas().style.cursor = "";
            });
          }
          setState("ready");
        });
      })
      .catch(() => {
        if (!cancelled) setState("error");
      });
    return () => {
      cancelled = true;
      resizeObserver?.disconnect();
      map?.remove();
      mapRef.current = null;
    };
  }, []);

  // Keep data and selection in sync.
  useEffect(() => {
    const map = mapRef.current;
    if (state !== "ready" || !map) return;
    (map.getSource(SOURCE) as GeoJSONSource | undefined)?.setData(toGeoJSON(countries));
  }, [countries, state]);

  useEffect(() => {
    const map = mapRef.current;
    if (state !== "ready" || !map) return;
    const primary = cssColor("--color-primary", "#2563eb");
    map.setPaintProperty("countries", "circle-color", ["case", ["==", ["get", "code"], selected ?? ""], "#0f172a", primary]);
    if (selected && COUNTRY_CENTROIDS[selected]) {
      map.easeTo({ center: COUNTRY_CENTROIDS[selected], zoom: Math.max(map.getZoom(), 2.6), duration: reducedMotion() ? 0 : 500 });
    }
  }, [selected, state]);

  return (
    <div className="relative h-full w-full overflow-hidden bg-[#eef2f6] dark:bg-slate-900">
      {/* Sized with h-full/w-full, not absolute inset-0: MapLibre's unlayered
          `.maplibregl-map { position: relative }` overrides Tailwind's layered
          `absolute`, which collapsed the container (and its canvas) to 0px. */}
      <div ref={containerRef} className="h-full w-full" aria-label={t("world.mapLabel")} role="region" />
      {state === "loading" ? (
        <div className="absolute inset-0 flex items-center justify-center" aria-busy="true">
          <span className="rounded-full bg-surface/90 px-3 py-1.5 text-xs font-medium text-muted shadow-sm">{t("world.mapLoading")}</span>
        </div>
      ) : null}
      {state === "error" ? (
        <div className="absolute inset-0 flex items-center justify-center p-6">
          <p className="max-w-xs rounded-2xl bg-surface/95 px-4 py-3 text-center text-sm text-muted shadow-sm">{t("world.mapError")}</p>
        </div>
      ) : null}
    </div>
  );
}
