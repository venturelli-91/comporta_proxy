import type { UseQueryResult } from "@tanstack/react-query";
import { botaoSecundario, Esqueleto } from "../ui";

// Estado mostra o esqueleto enquanto carrega ou o erro de uma query, com ação para tentar de novo.
export function Estado({ q }: { q: UseQueryResult<unknown> }) {
  if (q.isPending)
    return (
      <div className="space-y-6" role="status" aria-label="Carregando">
        <Esqueleto className="h-9 w-2/3" />
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5">
          {Array.from({ length: 5 }, (_, i) => (
            <Esqueleto key={i} className="h-20" />
          ))}
        </div>
        <Esqueleto className="h-56" />
      </div>
    );
  return (
    <div role="alert" className="flex flex-wrap items-center gap-3 rounded-lg border border-erro/30 bg-erro/8 px-4 py-3 text-sm">
      <span className="text-erro">Não foi possível carregar: {q.error?.message}.</span>
      <button className={botaoSecundario} onClick={() => q.refetch()}>
        Tentar de novo
      </button>
    </div>
  );
}
