import {
  createRouter,
  createRoute,
  createRootRoute,
  redirect,
  Outlet,
} from "@tanstack/react-router";
import { lazy } from "react";

import { AppShell } from "@/components/layout/AppShell";
import { useAuthStore } from "@/store/auth.store";
import { ErrorBoundary } from "@/components/ui/ErrorBoundary";

// Lazy-loaded pages for maximum code-splitting performance
const LoginPage             = lazy(() => import("@/pages/Login"));
const AcceptInvitePage      = lazy(() => import("@/pages/AcceptInvite"));
const DashboardPage         = lazy(() => import("@/pages/Dashboard"));
const CollectionsPage       = lazy(() => import("@/pages/Collections"));
const CollectionDetailPage  = lazy(() =>
  import("@/pages/Collections").then((m) => ({ default: m.CollectionDetailPage }))
);
const ContentOverviewPage = lazy(() => import("@/pages/ContentOverview"));
const ContentPage       = lazy(() => import("@/pages/Content"));
const ContentCreatePage = lazy(() =>
  import("@/pages/Content").then((m) => ({ default: m.ContentCreatePage }))
);
const ContentEditPage = lazy(() =>
  import("@/pages/Content").then((m) => ({ default: m.ContentEditPage }))
);
const UsersPage       = lazy(() => import("@/pages/Users"));
const TenantsPage     = lazy(() => import("@/pages/Tenants"));
const RolesPage       = lazy(() => import("@/pages/Roles"));
const ApiTokensPage   = lazy(() => import("@/pages/ApiTokens"));
const ProfilePage     = lazy(() => import("@/pages/Profile"));
const SettingsPage    = lazy(() => import("@/pages/Settings"));
const AuditPage       = lazy(() => import("@/pages/Audit"));
const WorkflowsPage   = lazy(() => import("@/pages/workflows"));
const WorkflowEditorPage = lazy(() =>
  import("@/pages/workflows").then((m) => ({ default: m.WorkflowEditorPage }))
);
const WorkflowRunsPage = lazy(() =>
  import("@/pages/workflows").then((m) => ({ default: m.WorkflowRunsPage }))
);
const NotFoundPage    = lazy(() => import("@/pages/NotFound"));

// ── Root route ─────────────────────────────────────────────────────────────────
const rootRoute = createRootRoute({
  component: () => <Outlet />,
});

// ── Auth guard ─────────────────────────────────────────────────────────────────
const authGuard = () => {
  const { isAuthenticated } = useAuthStore.getState();
  if (!isAuthenticated) {
    throw redirect({ to: "/login" });
  }
};

// ── Public routes ──────────────────────────────────────────────────────────────
const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/login",
  beforeLoad: () => {
    const { isAuthenticated } = useAuthStore.getState();
    if (isAuthenticated) throw redirect({ to: "/" });
  },
  component: () => <LoginPage />,
});

// Public — invited users set their password here (no auth required).
const acceptInviteRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/accept-invite",
  component: () => <AcceptInvitePage />,
});

// ── Protected layout route ────────────────────────────────────────────────────
const protectedRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: "protected",
  beforeLoad: authGuard,
  component: () => (
    <AppShell>
      <ErrorBoundary>
        <Outlet />
      </ErrorBoundary>
    </AppShell>
  ),
});

// ── App routes (inside AppShell) ──────────────────────────────────────────────
const indexRoute = createRoute({
  getParentRoute: () => protectedRoute,
  path: "/",
  component: () => <DashboardPage />,
});

const collectionsRoute = createRoute({
  getParentRoute: () => protectedRoute,
  path: "/collections",
  component: () => <CollectionsPage />,
});

const collectionDetailRoute = createRoute({
  getParentRoute: () => protectedRoute,
  path: "/collections/$name",
  component: () => <CollectionDetailPage />,
});

// Content overview — all collections as grouped cards, e.g. /content
const contentOverviewRoute = createRoute({
  getParentRoute: () => protectedRoute,
  path: "/content",
  component: () => <ContentOverviewPage />,
});

// Content browser — list view per collection, e.g. /content/blog_posts
const contentRoute = createRoute({
  getParentRoute: () => protectedRoute,
  path: "/content/$collection",
  component: () => <ContentPage />,
});

// Full-page create form, e.g. /content/blog_posts/create
const contentCreateRoute = createRoute({
  getParentRoute: () => protectedRoute,
  path: "/content/$collection/create",
  component: () => <ContentCreatePage />,
});

// Full-page edit form, e.g. /content/blog_posts/<id>/edit
const contentEditRoute = createRoute({
  getParentRoute: () => protectedRoute,
  path: "/content/$collection/$id/edit",
  component: () => <ContentEditPage />,
});

const workflowsRoute = createRoute({
  getParentRoute: () => protectedRoute,
  path: "/workflows",
  component: () => <WorkflowsPage />,
});

const workflowNewRoute = createRoute({
  getParentRoute: () => protectedRoute,
  path: "/workflows/new",
  component: () => <WorkflowEditorPage />,
});

const workflowDetailRoute = createRoute({
  getParentRoute: () => protectedRoute,
  path: "/workflows/$id",
  component: () => <WorkflowEditorPage />,
});

const workflowRunsRoute = createRoute({
  getParentRoute: () => protectedRoute,
  path: "/workflows/$id/runs",
  component: () => <WorkflowRunsPage />,
});

const usersRoute = createRoute({
  getParentRoute: () => protectedRoute,
  path: "/users",
  component: () => <UsersPage />,
});

const tenantsRoute = createRoute({
  getParentRoute: () => protectedRoute,
  path: "/tenants",
  component: () => <TenantsPage />,
});

const rolesRoute = createRoute({
  getParentRoute: () => protectedRoute,
  path: "/roles",
  component: () => <RolesPage />,
});

const apiTokensRoute = createRoute({
  getParentRoute: () => protectedRoute,
  path: "/api-tokens",
  component: () => <ApiTokensPage />,
});

const profileRoute = createRoute({
  getParentRoute: () => protectedRoute,
  path: "/profile",
  component: () => <ProfilePage />,
});

const settingsRoute = createRoute({
  getParentRoute: () => protectedRoute,
  path: "/settings",
  component: () => <SettingsPage />,
});

const auditRoute = createRoute({
  getParentRoute: () => protectedRoute,
  path: "/audit",
  component: () => <AuditPage />,
});

const notFoundRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "*",
  component: () => <NotFoundPage />,
});

// ── Route tree ────────────────────────────────────────────────────────────────
const routeTree = rootRoute.addChildren([
  loginRoute,
  acceptInviteRoute,
  protectedRoute.addChildren([
    indexRoute,
    collectionsRoute,
    collectionDetailRoute,
    contentOverviewRoute, // /content (index) — before $collection
    contentCreateRoute,   // must come before contentRoute so /create isn't swallowed by $collection
    contentEditRoute,
    contentRoute,
    workflowNewRoute,     // before $id
    workflowRunsRoute,    // before $id so /runs isn't swallowed
    workflowDetailRoute,
    workflowsRoute,
    usersRoute,
    tenantsRoute,
    rolesRoute,
    apiTokensRoute,
    profileRoute,
    settingsRoute,
    auditRoute,
  ]),
  notFoundRoute,
]);

export const router = createRouter({
  routeTree,
  defaultPreload: "intent",          // preload on hover — fast navigation
  defaultPreloadDelay: 50,
  defaultStaleTime: 5_000,
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
