import { MutationCache, QueryCache, QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import { ErroApi } from "./api";
import "./index.css";

// Sessão vencida (12 h) ou revogada no servidor: qualquer 401 de uma tela refaz a consulta da
// sessão, que também dá 401, e o App volta para o login em vez de deixar as telas quebradas.
// A própria consulta da sessão fica de fora, senão o 401 dela reiniciaria ela mesma em laço.
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
