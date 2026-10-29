import type { Messages } from "./en";

export const de: Messages = {
  app: {
    tagline: "Wer wohin gehört, wer an wen berichtet und wer wofür verantwortlich ist.",
  },
  nav: {
    main: "Hauptnavigation",
    overview: "Übersicht",
    organisation: "Organisation",
    people: "Personen",
    processes: "Prozesse",
    collapse: "Seitenleiste einklappen",
    expand: "Seitenleiste ausklappen",
    search: "Personen, Rollen, Einheiten und Aufgaben suchen",
  },
  home: {
    title: "Übersicht",
    intro: "Bearing bildet Ihre Organisation aus Einheiten, Stellen, Rollen und Personen ab und zeichnet ihre Prozesse mit der Verantwortung für jeden Schritt.",
    next: "Was als Nächstes kommt",
    steps: [
      "Das Organisationsmodell: Einheiten, Stellen, Berichtslinien und die Personen, die sie besetzen.",
      "Das Organigramm, Personenseiten und die Suche.",
      "Prozesse als Flussgraphen, jeder Schritt mit seinen Verantwortlichkeiten.",
    ],
    status: "Diese Installation",
    apiBuild: (version: string) => `API ${version}`,
    apiUnreachable: "Die API antwortet nicht.",
  },
  account: {
    menu: "Konto und Einstellungen",
    language: "Sprache",
    languageBrowser: "Wie der Browser",
    languages: { en: "English", de: "Deutsch" },
    about: (version: string) => `Bearing ${version}`,
  },
  comingSoon: {
    body: "Dieser Teil von Bearing ist geplant und noch nicht gebaut.",
  },
  notFound: {
    title: "Hier ist nichts",
    body: "Diese Adresse führt in Bearing nirgendwohin.",
    back: "Zurück zur Übersicht",
  },
};
