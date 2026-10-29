import type { QueryClient } from "@tanstack/react-query";
import { createRootRouteWithContext, createRoute, createRouter, type RouterHistory } from "@tanstack/react-router";
import { Home } from "@/features/home/Home";
import { AppShell } from "@/features/shell/AppShell";
import { ComingSoon, NotFound } from "@/features/shell/Placeholder";
import { t } from "@/i18n";

type RouterContext = { queryClient: QueryClient };

const rootRoute = createRootRouteWithContext<RouterContext>()({ component: AppShell, notFoundComponent: NotFound });

const homeRoute = createRoute({ getParentRoute: () => rootRoute, path: "/", component: Home });
const organisationRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/organisation",
  component: () => <ComingSoon title={() => t.nav.organisation} />,
});
const peopleRoute = createRoute({ getParentRoute: () => rootRoute, path: "/people", component: () => <ComingSoon title={() => t.nav.people} /> });
const processesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/processes",
  component: () => <ComingSoon title={() => t.nav.processes} />,
});

const routeTree = rootRoute.addChildren([homeRoute, organisationRoute, peopleRoute, processesRoute]);

export function buildRouter(queryClient: QueryClient, history?: RouterHistory) {
  return createRouter({ routeTree, context: { queryClient }, history, defaultPreload: "intent" });
}

declare module "@tanstack/react-router" {
  interface Register {
    router: ReturnType<typeof buildRouter>;
  }
}
