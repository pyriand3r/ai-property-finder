<script setup lang="ts">
import 'ol/ol.css';

import { Feature, Map as OlMap, Overlay, View } from 'ol';
import { boundingExtent } from 'ol/extent';
import { Point } from 'ol/geom';
import TileLayer from 'ol/layer/Tile';
import VectorLayer from 'ol/layer/Vector';
import { fromLonLat } from 'ol/proj';
import OSM from 'ol/source/OSM';
import VectorSource from 'ol/source/Vector';
import { Circle as CircleStyle, Fill, Stroke, Style } from 'ol/style';
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue';

import { useListingThumbnails } from '@/composables/useListingThumbnails';
import { statusColor } from '@/utils/listingStatus';

import type { Listing } from '@/types/listing';
import type { FeatureLike } from 'ol/Feature';

const props = defineProps<{
  listings: readonly Listing[];
  selectedListingId?: string | null;
}>();

const emit = defineEmits<{
  select: [listingId: string];
}>();

// Only listings with both coordinates set can appear on the map — no
// coordinates means the Agent couldn't geocode the address (or none was
// present); the List View remains the complete record (spec 01-06).
const geocodedListings = computed(() =>
  props.listings.filter(
    (listing) => listing.latitude !== undefined && listing.longitude !== undefined,
  ),
);

const { thumbnails } = useListingThumbnails(() => props.listings);

const mapContainer = ref<HTMLDivElement | null>(null);
const popupContainer = ref<HTMLDivElement | null>(null);
const popupListing = shallowRef<Listing | null>(null);

let map: OlMap | null = null;
let overlay: Overlay | null = null;
let vectorSource: VectorSource | null = null;
let vectorLayer: VectorLayer | null = null;
let currentSelectedId: string | null = null;

// Popup opens on hover after a short delay (not immediately on hover, and
// not on click at all — click only selects the Listing for the right
// pane). The hide delay lets the pointer travel from the marker onto the
// popup itself without it flickering shut in between.
const HOVER_OPEN_DELAY_MS = 400;
const HOVER_CLOSE_DELAY_MS = 150;
let hoveredListingId: string | null = null;
let hoverOpenTimer: ReturnType<typeof setTimeout> | null = null;
let hoverCloseTimer: ReturnType<typeof setTimeout> | null = null;
let pointerOverPopup = false;

function clearHoverTimers() {
  if (hoverOpenTimer !== null) {
    clearTimeout(hoverOpenTimer);
    hoverOpenTimer = null;
  }
  if (hoverCloseTimer !== null) {
    clearTimeout(hoverCloseTimer);
    hoverCloseTimer = null;
  }
}

function openPopup(listing: Listing, feature: Feature) {
  popupListing.value = listing;
  overlay!.setPosition((feature.getGeometry() as Point).getCoordinates());
}

function scheduleClosePopup() {
  hoverCloseTimer = setTimeout(() => {
    if (!pointerOverPopup) {
      closePopup();
    }
  }, HOVER_CLOSE_DELAY_MS);
}

function markerStyle(feature: FeatureLike): Style {
  const listing = feature.get('listing') as Listing;
  const isSelected = listing.id === currentSelectedId;
  return new Style({
    image: new CircleStyle({
      radius: isSelected ? 10 : 7,
      fill: new Fill({ color: statusColor[listing.status] }),
      stroke: new Stroke({
        color: isSelected ? '#ffffff' : '#1a1a1a',
        width: isSelected ? 3 : 1.5,
      }),
    }),
  });
}

function buildFeatures(): Feature[] {
  return geocodedListings.value.map((listing) => {
    const feature = new Feature({
      geometry: new Point(fromLonLat([listing.longitude!, listing.latitude!])),
    });
    feature.set('listing', listing);
    return feature;
  });
}

function fitToMarkers() {
  if (!map || geocodedListings.value.length === 0) {
    return;
  }
  const coords = geocodedListings.value.map((listing) =>
    fromLonLat([listing.longitude!, listing.latitude!]),
  );
  map.getView().fit(boundingExtent(coords), { padding: [40, 40, 40, 40], maxZoom: 14 });
}

onMounted(() => {
  if (!mapContainer.value || !popupContainer.value) {
    return;
  }

  vectorSource = new VectorSource({ features: buildFeatures() });
  vectorLayer = new VectorLayer({ source: vectorSource, style: markerStyle });

  overlay = new Overlay({
    element: popupContainer.value,
    positioning: 'bottom-center',
    offset: [0, -12],
    stopEvent: true,
  });

  // Default view: central Sweden — most seeded listings are Swedish, and
  // fitToMarkers() below overrides this immediately whenever any listing
  // has coordinates.
  map = new OlMap({
    target: mapContainer.value,
    layers: [new TileLayer({ source: new OSM() }), vectorLayer],
    overlays: [overlay],
    view: new View({ center: fromLonLat([15, 62]), zoom: 4 }),
  });

  fitToMarkers();

  map.on('click', (event) => {
    const feature = map!.forEachFeatureAtPixel(event.pixel, (found) => found);
    if (feature) {
      emit('select', (feature.get('listing') as Listing).id);
    }
  });

  map.on('pointermove', (event) => {
    const feature = map!.forEachFeatureAtPixel(event.pixel, (found) => found) as
      | Feature
      | undefined;
    const listing = feature?.get('listing') as Listing | undefined;
    map!.getTargetElement().style.cursor = listing ? 'pointer' : '';

    if (listing?.id === hoveredListingId) {
      return;
    }
    hoveredListingId = listing?.id ?? null;
    clearHoverTimers();

    if (listing && feature) {
      hoverOpenTimer = setTimeout(() => openPopup(listing, feature), HOVER_OPEN_DELAY_MS);
    } else {
      scheduleClosePopup();
    }
  });

  popupContainer.value.addEventListener('mouseenter', () => {
    pointerOverPopup = true;
    clearHoverTimers();
  });
  popupContainer.value.addEventListener('mouseleave', () => {
    pointerOverPopup = false;
    scheduleClosePopup();
  });
});

onBeforeUnmount(() => {
  clearHoverTimers();
  map?.setTarget(undefined);
  map = null;
});

watch(geocodedListings, () => {
  vectorSource?.clear();
  vectorSource?.addFeatures(buildFeatures());
  fitToMarkers();
});

watch(
  () => props.selectedListingId,
  (id) => {
    currentSelectedId = id ?? null;
    vectorLayer?.changed();
  },
);

function closePopup() {
  popupListing.value = null;
  hoveredListingId = null;
  overlay?.setPosition(undefined);
}

function handleCloseClick() {
  clearHoverTimers();
  closePopup();
}
</script>

<template>
  <div class="listing-map">
    <div ref="mapContainer" class="listing-map-canvas"></div>
    <div ref="popupContainer" class="listing-map-popup" :class="{ 'is-visible': popupListing }">
      <template v-if="popupListing">
        <button type="button" class="listing-map-popup-close" @click="handleCloseClick">
          &times;
        </button>
        <img
          v-if="thumbnails.get(popupListing.id)"
          :src="thumbnails.get(popupListing.id) ?? ''"
          class="listing-map-popup-thumb"
          alt=""
        />
        <div v-else class="listing-map-popup-thumb-placeholder"></div>
        <div class="listing-map-popup-body">
          <strong>{{ popupListing.title }}</strong>
          <span v-if="popupListing.price">
            {{ popupListing.price.toLocaleString('de-DE') }} {{ popupListing.price_currency }}
          </span>
          <span v-if="popupListing.overall_score !== undefined">
            Score {{ popupListing.overall_score.toFixed(0) }}
          </span>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.listing-map {
  position: relative;
}

.listing-map-canvas {
  width: 100%;
  height: calc(100vh - 12rem);
  min-height: 400px;
  border-radius: 0.5rem;
  overflow: hidden;
}

/* Popup content is moved by OL's Overlay into the map's own DOM subtree, so
   it still sits inside NConfigProvider's tree and inherits Naive UI's
   internal --n-* custom properties — but those are Naive's own component
   theming variables (e.g. --n-color is a component's *primary* color, not
   a general surface color) and aren't meant to be borrowed by arbitrary
   elements. Styled directly against prefers-color-scheme instead, which is
   what usePreferredDark.ts also keys off. */
.listing-map-popup {
  display: none;
  position: absolute;
  background: #ffffff;
  color: #1a1a1a;
  border: 1px solid rgba(0, 0, 0, 0.15);
  border-radius: 0.5rem;
  padding: 0.75rem;
  min-width: 220px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.3);
  gap: 0.75rem;
}

@media (prefers-color-scheme: dark) {
  .listing-map-popup {
    background: #232323;
    color: #f0f0f0;
    border-color: rgba(255, 255, 255, 0.15);
  }
}

.listing-map-popup.is-visible {
  display: flex;
}

.listing-map-popup-close {
  position: absolute;
  top: 0.25rem;
  right: 0.5rem;
  background: none;
  border: none;
  cursor: pointer;
  font-size: 1.1rem;
  line-height: 1;
  color: inherit;
  opacity: 0.7;
}

.listing-map-popup-close:hover {
  opacity: 1;
}

.listing-map-popup-thumb {
  width: 100px;
  height: 100px;
  border-radius: 0.375rem;
  object-fit: cover;
  flex-shrink: 0;
}

.listing-map-popup-thumb-placeholder {
  width: 100px;
  height: 100px;
  border-radius: 0.375rem;
  background: var(--n-color-target, rgba(128, 128, 128, 0.15));
  flex-shrink: 0;
}

.listing-map-popup-body {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  padding-right: 1rem;
}
</style>
