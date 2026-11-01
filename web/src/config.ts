/** Tunable values shared across the client. */

/** The product's name, as the title bar and the sidebar show it. */
export const APP_NAME = "Bearing";

/** The release this client was built from, read from the root VERSION file at build time. */
export const APP_VERSION: string = __APP_VERSION__;

/** Where the API lives, relative to the page; the dev server and nginx proxy it. */
export const API_BASE = "/api/v1";

/** Where the language a person chose is remembered in the browser. */
export const LANGUAGE_STORAGE_KEY = "bearing.language";

/** Where the light or dark look a person chose is remembered in the browser. */
export const MODE_STORAGE_KEY = "bearing.mode";

/** Where the compiled theme this browser last showed is kept, so the next load paints it before the server answers. */
export const THEME_CACHE_KEY = "bearing.theme-css";
/** The id of the stylesheet a theme is put on the page as. */
export const THEME_STYLE_ID = "bearing-theme";

/** Where the sidebar's folded state is remembered in the browser. */
export const SIDEBAR_STORAGE_KEY = "bearing.sidebar";

/** The sidebar, open and as the icon rail it folds to, in pixels. */
export const SIDEBAR_WIDTH = 232;
export const SIDEBAR_RAIL_WIDTH = 56;

/** Below this viewport width the sidebar starts as a rail, unless the person chose otherwise. */
export const SIDEBAR_RAIL_BELOW_PX = 1024;

/** Where the org chart remembers the cards a person moved, so the layout keeps them where they were put. */
export const CHART_PLACES_STORAGE_KEY = "bearing.chart.places";

/** The largest file a theme takes, as the server says, so the editor refuses it before sending. */
export const THEME_ASSET_MAX_BYTES = 4 * 1024 * 1024;
/** The extra CSS a theme may carry, in bytes. */
export const THEME_CSS_MAX_BYTES = 32 * 1024;
/** How long the editor waits after a change before restyling the page with the draft. */
export const THEME_PREVIEW_DEBOUNCE_MS = 150;
/** The editor's limits for corners and a cursor's point, in pixels, and the size a cursor picture should be. */
export const THEME_MAX_RADIUS = 32;
export const THEME_MAX_HOTSPOT = 128;
export const THEME_CURSOR_PX = 32;
/** What an empty colour or shadow field shows when the stylesheet has no value to offer. */
export const THEME_COLOUR_PLACEHOLDER = "#000000";
export const THEME_SHADOW_PLACEHOLDER = "0 1px 2px rgb(0 0 0 / 0.1)";
export const KILOBYTE = 1024;

/** How many pieces a click throws, how long each lives, and how hard they fall. */
export const CONFETTI_PIECES_PER_CLICK = 28;
export const CONFETTI_LIFE_MS = 1100;
export const CONFETTI_GRAVITY_PX_PER_S2 = 1600;
/** The colours of the paper scraps, and the characters thrown with them, as Stator and Exciter throw them. */
export const CONFETTI_COLOURS = ["#ff1744", "#ffab00", "#00c853", "#2979ff", "#d500f9", "#00e5ff", "#ff2e88", "#ffd600"];
export const CONFETTI_GLYPHS = ["\u{1F389}", "\u{2728}", "\u{1F9E7}", "\u{1F308}", "\u{1F4A5}", "\u{1F340}", "\u{1F38A}", "\u{2B50}"];

/** How many wandering points the constellation backdrop draws per million pixels of page. */
export const CONSTELLATION_POINTS_PER_MEGAPIXEL = 70;
/** How close two of them have to be for a line to join them, in pixels. */
export const CONSTELLATION_LINK_PX = 150;
/** How fast a point wanders, in pixels per second, before its own variation. */
export const CONSTELLATION_SPEED_PX_PER_S = 14;
/** The grey a point or a line is drawn in when the theme's colour cannot be read. */
export const CONSTELLATION_FALLBACK_COLOUR = "#808080";

/** How many attempts a webhook's log shows, newest first; the API keeps them for 30 days. */
export const WEBHOOK_DELIVERIES_PAGE_SIZE = 50;
