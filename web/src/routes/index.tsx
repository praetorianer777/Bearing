import type { QueryClient } from "@tanstack/react-query";
import { createRootRouteWithContext, createRoute, createRouter, Outlet, redirect, type RouterHistory } from "@tanstack/react-router";
import { installationQuery, meQuery } from "@/api/auth";
import { AcceptInvitationPage, ForgotPasswordPage, ResetPasswordPage, SetUpPage, SignInPage } from "@/features/auth/pages";
import { Home } from "@/features/home/Home";
import { AccountsPage } from "@/features/settings/Accounts";
import { AppShell } from "@/features/shell/AppShell";
import { ComingSoon, NotFound } from "@/features/shell/Placeholder";
import { t } from "@/i18n";

type RouterContext = { queryClient: QueryClient };

const rootRoute = createRootRouteWithContext<RouterContext>()({ component: Outlet, notFoundComponent: NotFound });

const setUpRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/setup",
  beforeLoad: async ({ context }) => {
    if ((await context.queryClient.ensureQueryData(installationQuery)).setUp) throw redirect({ to: "/sign-in" });
  },
  component: SetUpPage,
});

const signInRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/sign-in",
  validateSearch: (search: Record<string, unknown>): { next?: string } => (typeof search.next === "string" ? { next: search.next } : {}),
  beforeLoad: async ({ context }) => {
    if (!(await context.queryClient.ensureQueryData(installationQuery)).setUp) throw redirect({ to: "/setup" });
    if (await context.queryClient.ensureQueryData(meQuery)) throw redirect({ to: "/" });
  },
  component: function SignIn() {
    return <SignInPage next={signInRoute.useSearch().next} />;
  },
});

const forgotRoute = createRoute({ getParentRoute: () => rootRoute, path: "/forgot-password", component: ForgotPasswordPage });

const resetRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/reset-password/$token",
  component: function Reset() {
    return <ResetPasswordPage token={resetRoute.useParams().token} />;
  },
});

const invitationRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/invitations/$token",
  component: function Accept() {
    return <AcceptInvitationPage token={invitationRoute.useParams().token} />;
  },
});

// Everything inside the shell needs somebody signed in to an installation that is set up.
const appRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: "app",
  beforeLoad: async ({ context, location }) => {
    if (!(await context.queryClient.ensureQueryData(installationQuery)).setUp) throw redirect({ to: "/setup" });
    const me = await context.queryClient.ensureQueryData(meQuery);
    if (!me) throw redirect({ to: "/sign-in", search: { next: location.href } });
    return { me };
  },
  component: AppShell,
});

const homeRoute = createRoute({ getParentRoute: () => appRoute, path: "/", component: Home });
const organisationRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/organisation",
  component: () => <ComingSoon title={() => t.nav.organisation} />,
});
const peopleRoute = createRoute({ getParentRoute: () => appRoute, path: "/people", component: () => <ComingSoon title={() => t.nav.people} /> });
const processesRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/processes",
  component: () => <ComingSoon title={() => t.nav.processes} />,
});
const accountsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/settings/accounts",
  beforeLoad: ({ context }) => {
    if (context.me.role !== "administrator") throw redirect({ to: "/" });
  },
  component: AccountsPage,
});

const routeTree = rootRoute.addChildren([
  setUpRoute,
  signInRoute,
  forgotRoute,
  resetRoute,
  invitationRoute,
  appRoute.addChildren([homeRoute, organisationRoute, peopleRoute, processesRoute, accountsRoute]),
]);

export function buildRouter(queryClient: QueryClient, history?: RouterHistory) {
  return createRouter({ routeTree, context: { queryClient }, history, defaultPreload: "intent" });
}

declare module "@tanstack/react-router" {
  interface Register {
    router: ReturnType<typeof buildRouter>;
  }
}
