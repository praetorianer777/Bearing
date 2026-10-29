import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";

// Stamps the light or dark look this browser chose before anything renders, so no page flashes the other one.
import "./lib/appearance";
import { buildRouter } from "./routes";
import "./styles/index.css";

const queryClient = new QueryClient({ defaultOptions: { queries: { retry: 1, staleTime: 30_000 } } });
const router = buildRouter(queryClient);

const container = document.getElementById("root");
if (!container) throw new Error("root element is missing from index.html");

createRoot(container).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </StrictMode>,
);
