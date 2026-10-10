import type { UseQueryResult } from "@tanstack/react-query";

// Estado mostra carregando ou erro de uma query, com ação para tentar de novo.
export function Estado({ q }: { q: UseQueryResult<unknown> }) {
  if (q.isPending) return <p className="text-suave">Carregando…</p>;
  return (
    <p role="alert" className="text-erro">
      Não foi possível carregar: {q.error?.message}.{" "}
      <button className="underline" onClick={() => q.refetch()}>
        Tentar de novo
      </button>
    </p>
  );
}
