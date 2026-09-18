import React from "react";
import ReactDOM from "react-dom/client";
import { RouterProvider } from "@tanstack/react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ReactQueryDevtools } from "@tanstack/react-query-devtools";

import { router } from "@/routes";
import { initTheme } from "@/store/theme.store";
import "@/styles/globals.css";

// Apply persisted light/dark theme before first paint (no flash).
initTheme();

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,       // 30 s
      gcTime: 5 * 60_000,      // 5 min
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
});

// The auth session (token + user) rehydrates from localStorage via the persisted
// Zustand store, so the app knows on first render whether the user is signed in.
ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
      {import.meta.env.DEV && <ReactQueryDevtools buttonPosition="bottom-right" />}
    </QueryClientProvider>
  </React.StrictMode>
);
