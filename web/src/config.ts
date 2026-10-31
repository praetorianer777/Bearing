/** Tunable values shared across the client. */

/** The product's name, as the title bar and the sidebar show it. */
export const APP_NAME = "Bearing";

/** The release this client was built from, read from the root VERSION file at build time. */
export const APP_VERSION: string = __APP_VERSION__;

/** Where the API lives, relative to the page; the dev server and nginx proxy it. */
export const API_BASE = "/api/v1";

/** Where the language a person chose is remembered in the browser. */
export const LANGUAGE_STORAGE_KEY = "bearing.language";

/** Where the light or dark look a person chose, and the theme, are remembered in the browser. */
export const MODE_STORAGE_KEY = "bearing.mode";
export const THEME_STORAGE_KEY = "bearing.theme";

/** Where the sidebar's folded state is remembered in the browser. */
export const SIDEBAR_STORAGE_KEY = "bearing.sidebar";

/** The sidebar, open and as the icon rail it folds to, in pixels. */
export const SIDEBAR_WIDTH = 232;
export const SIDEBAR_RAIL_WIDTH = 56;

/** Below this viewport width the sidebar starts as a rail, unless the person chose otherwise. */
export const SIDEBAR_RAIL_BELOW_PX = 1024;

/** Where the org chart remembers the cards a person moved, so the layout keeps them where they were put. */
export const CHART_PLACES_STORAGE_KEY = "bearing.chart.places";
