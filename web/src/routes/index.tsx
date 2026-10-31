import type { QueryClient } from "@tanstack/react-query";
import { createRootRouteWithContext, createRoute, createRouter, Outlet, redirect, type RouterHistory } from "@tanstack/react-router";
import { installationQuery, meQuery } from "@/api/auth";
import { AcceptInvitationPage, ForgotPasswordPage, ResetPasswordPage, SetUpPage, SignInPage } from "@/features/auth/pages";
import { type ChartSearch, OrgChartPage } from "@/features/chart/OrgChart";
import { Home } from "@/features/home/Home";
import { PositionPage } from "@/features/org/PositionPage";
import { RolePage } from "@/features/org/RolePage";
import { type DaySearch, UnitPage } from "@/features/org/UnitPage";
import { type PeopleSearch, PeoplePage } from "@/features/people/PeoplePage";
import { type PersonSearch, PersonPage } from "@/features/people/PersonPage";
import { type ProcessSearch, ProcessPage } from "@/features/processes/ProcessPage";
import { ProcessesPage } from "@/features/processes/ProcessesPage";
import { type WhoDoesSearch, WhoDoesPage } from "@/features/processes/WhoDoesPage";
import { AccountsPage } from "@/features/settings/Accounts";
import { ExampleDataPage } from "@/features/settings/ExampleData";
import { ProfilePage } from "@/features/settings/Profile";
import { ThemesPage } from "@/features/settings/Themes";
import { UnitTypesPage } from "@/features/settings/UnitTypes";
import { AppShell } from "@/features/shell/AppShell";
import { NotFound } from "@/features/shell/Placeholder";

type RouterContext = { queryClient: QueryClient };

/** Whether an address's value is a day written 2006-01-02. */
const isDay = (value: unknown): value is string => typeof value === "string" && /^\d{4}-\d{2}-\d{2}$/.test(value);

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
  validateSearch: (search: Record<string, unknown>): { next?: string; sso?: string } => ({
    ...(typeof search.next === "string" ? { next: search.next } : {}),
    ...(typeof search.sso === "string" ? { sso: search.sso } : {}),
  }),
  beforeLoad: async ({ context }) => {
    if (!(await context.queryClient.ensureQueryData(installationQuery)).setUp) throw redirect({ to: "/setup" });
    if (await context.queryClient.ensureQueryData(meQuery)) throw redirect({ to: "/" });
  },
  component: function SignIn() {
    const { next, sso } = signInRoute.useSearch();
    return <SignInPage next={next} sso={sso} />;
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
  validateSearch: (search: Record<string, unknown>): ChartSearch => ({
    ...(isDay(search.asOf) ? { asOf: search.asOf } : {}),
    ...(typeof search.focus === "string" ? { focus: search.focus } : {}),
  }),
  component: function Organisation() {
    const search = organisationRoute.useSearch();
    const navigate = organisationRoute.useNavigate();
    return <OrgChartPage search={search} onSearch={(next) => navigate({ search: next })} />;
  },
});
const peopleRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/people",
  validateSearch: (search: Record<string, unknown>): PeopleSearch => ({
    ...(typeof search.q === "string" && search.q ? { q: search.q } : {}),
    ...(search.left === true || search.left === "true" ? { left: true } : {}),
  }),
  component: function People() {
    const search = peopleRoute.useSearch();
    const navigate = peopleRoute.useNavigate();
    return <PeoplePage search={search} onSearch={(next) => navigate({ search: next, replace: true })} />;
  },
});
const personRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/people/$personId",
  validateSearch: (search: Record<string, unknown>): PersonSearch => (isDay(search.asOf) ? { asOf: search.asOf } : {}),
  component: function Person() {
    const { personId } = personRoute.useParams();
    const search = personRoute.useSearch();
    const navigate = personRoute.useNavigate();
    return <PersonPage personId={personId} search={search} onSearch={(next) => navigate({ search: next })} />;
  },
});
const processesRoute = createRoute({ getParentRoute: () => appRoute, path: "/processes", component: ProcessesPage });
const processRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/processes/$processId",
  validateSearch: (search: Record<string, unknown>): ProcessSearch => {
    const version = Number(search.version);
    return {
      ...(Number.isInteger(version) && version >= 1 ? { version } : {}),
      ...(search.view === "matrix" || search.view === "lanes" ? { view: search.view } : {}),
      ...(isDay(search.asOf) ? { asOf: search.asOf } : {}),
    };
  },
  component: function Process() {
    const { processId } = processRoute.useParams();
    const search = processRoute.useSearch();
    const navigate = processRoute.useNavigate();
    return <ProcessPage processId={processId} search={search} onSearch={(next) => navigate({ search: next })} />;
  },
});
const whoDoesRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/who-does",
  validateSearch: (search: Record<string, unknown>): WhoDoesSearch => ({
    ...(typeof search.q === "string" && search.q ? { q: search.q } : {}),
    ...(isDay(search.asOf) ? { asOf: search.asOf } : {}),
  }),
  component: function WhoDoes() {
    const search = whoDoesRoute.useSearch();
    const navigate = whoDoesRoute.useNavigate();
    return <WhoDoesPage search={search} onSearch={(next) => navigate({ search: next, replace: true })} />;
  },
});
const roleRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/roles/$roleId",
  validateSearch: (search: Record<string, unknown>): DaySearch => (isDay(search.asOf) ? { asOf: search.asOf } : {}),
  component: function Role() {
    const { roleId } = roleRoute.useParams();
    const search = roleRoute.useSearch();
    const navigate = roleRoute.useNavigate();
    return <RolePage roleId={roleId} search={search} onSearch={(next) => navigate({ search: next })} />;
  },
});
const unitRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/units/$unitId",
  validateSearch: (search: Record<string, unknown>): DaySearch => (isDay(search.asOf) ? { asOf: search.asOf } : {}),
  component: function Unit() {
    const { unitId } = unitRoute.useParams();
    const search = unitRoute.useSearch();
    const navigate = unitRoute.useNavigate();
    return <UnitPage unitId={unitId} search={search} onSearch={(next) => navigate({ search: next })} />;
  },
});
const positionRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/positions/$positionId",
  validateSearch: (search: Record<string, unknown>): DaySearch => (isDay(search.asOf) ? { asOf: search.asOf } : {}),
  component: function Position() {
    const { positionId } = positionRoute.useParams();
    const search = positionRoute.useSearch();
    const navigate = positionRoute.useNavigate();
    return <PositionPage positionId={positionId} search={search} onSearch={(next) => navigate({ search: next })} />;
  },
});
const accountsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/settings/accounts",
  beforeLoad: ({ context }) => {
    if (context.me.role !== "administrator") throw redirect({ to: "/" });
  },
  component: AccountsPage,
});
const themesRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/settings/themes",
  beforeLoad: ({ context }) => {
    if (context.me.role !== "administrator") throw redirect({ to: "/" });
  },
  component: ThemesPage,
});
const unitTypesRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/settings/unit-types",
  beforeLoad: ({ context }) => {
    if (context.me.role !== "administrator") throw redirect({ to: "/" });
  },
  component: UnitTypesPage,
});
const exampleDataRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/settings/example-data",
  beforeLoad: ({ context }) => {
    if (context.me.role !== "administrator") throw redirect({ to: "/" });
  },
  component: ExampleDataPage,
});
const profileRoute = createRoute({ getParentRoute: () => appRoute, path: "/settings/profile", component: ProfilePage });

const routeTree = rootRoute.addChildren([
  setUpRoute,
  signInRoute,
  forgotRoute,
  resetRoute,
  invitationRoute,
  appRoute.addChildren([
    homeRoute,
    organisationRoute,
    peopleRoute,
    personRoute,
    unitRoute,
    positionRoute,
    processesRoute,
    processRoute,
    whoDoesRoute,
    roleRoute,
    accountsRoute,
    themesRoute,
    unitTypesRoute,
    exampleDataRoute,
    profileRoute,
  ]),
]);

export function buildRouter(queryClient: QueryClient, history?: RouterHistory) {
  return createRouter({ routeTree, context: { queryClient }, history, defaultPreload: "intent" });
}

declare module "@tanstack/react-router" {
  interface Register {
    router: ReturnType<typeof buildRouter>;
  }
}
