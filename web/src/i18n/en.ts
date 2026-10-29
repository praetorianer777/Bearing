import type { Language } from "./index";

export const en = {
  app: {
    tagline: "Who belongs where, who reports to whom, and who is responsible for what.",
  },
  nav: {
    main: "Main",
    overview: "Overview",
    organisation: "Organisation",
    people: "People",
    processes: "Processes",
    collapse: "Fold the sidebar",
    expand: "Unfold the sidebar",
    search: "Search people, roles, units and tasks",
  },
  home: {
    title: "Overview",
    intro: "Bearing models your organisation as units, positions, roles and people, and draws its processes with who is responsible for each step.",
    next: "What comes next",
    steps: [
      "The organisation model: units, positions, reporting lines and the people who hold them.",
      "The org chart, person pages and search.",
      "Processes drawn as flow graphs, each step with its responsibilities.",
    ],
    status: "This installation",
    apiBuild: (version: string) => `API ${version}`,
    apiUnreachable: "The API does not answer.",
  },
  account: {
    menu: "Account and settings",
    language: "Language",
    languageBrowser: "As the browser",
    languages: { en: "English", de: "Deutsch" } satisfies Record<Language, string>,
    about: (version: string) => `Bearing ${version}`,
  },
  comingSoon: {
    body: "This part of Bearing is planned and not built yet.",
  },
  notFound: {
    title: "Nothing here",
    body: "This address leads nowhere in Bearing.",
    back: "Back to the overview",
  },
};

export type Messages = typeof en;
