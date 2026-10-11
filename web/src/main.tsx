import { MutationCache, QueryCache, QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import { ErroApi } from "./api";
import "./index.css";

// Session expired (12 h) or revoked on the server: any 401 on a screen refetches the session
// query, which also returns 401, and the App goes back to login instead of leaving broken screens.
// The session query itself is left out, otherwise its own 401 would restart it in a loop.
const sessaoCaiu = (err: Error, chave?: readonly unknown[]) => {
  if (err instanceof ErroApi && err.status === 401 && chave?.[0] !== "sessao") {
    cliente.resetQueries({ queryKey: ["sessao"] });
  }
};

const cliente = new QueryClient({
  defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false } },
  queryCache: new QueryCache({ onError: (err, query) => sessaoCaiu(err, query.queryKey) }),
  mutationCache: new MutationCache({ onError: (err) => sessaoCaiu(err) }),
});

createRoot(document.getElementById("raiz")!).render(
  <StrictMode>
    <QueryClientProvider client={cliente}>
      <App />
    </QueryClientProvider>
  </StrictMode>,
);
