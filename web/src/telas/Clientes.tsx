import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { api, enviar, usd, type Chave, type Cliente, type Modelo } from "../api";
import { Estado } from "./Estado";

const campo = "mt-1 block w-full rounded-md border border-borda bg-superficie px-3 py-2";

export default function Clientes() {
  const clientes = useQuery({ queryKey: ["clientes"], queryFn: () => api<Cliente[]>("/admin/clientes") });
  const [aberto, setAberto] = useState<number | null>(null);
  if (!clientes.data) return <Estado q={clientes} />;
  return (
    <div className="grid gap-10 lg:grid-cols-[1fr_20rem]">
      <section aria-labelledby="lista">
        <h2 id="lista" className="mb-3 font-medium">Clientes</h2>
        {clientes.data.length === 0 ? (
          <p className="text-suave">Nenhum cliente ainda. Cadastre o primeiro ao lado para gerar uma chave.</p>
        ) : (
          <ul className="divide-y divide-borda rounded-lg border border-borda bg-superficie">
            {clientes.data.map((c) => (
              <li key={c.id} className="px-4 py-3">
                <button
                  className="flex w-full flex-wrap items-baseline justify-between gap-2 text-left"
                  aria-expanded={aberto === c.id}
                  onClick={() => setAberto(aberto === c.id ? null : c.id)}
                >
                  <span className="font-medium">{c.nome}</span>
                  <span className="text-sm text-suave">
                    {c.rpm ? `${c.rpm} req/min` : "sem limite"} ·{" "}
                    {c.orcamento_micro ? `${usd(c.orcamento_micro)}/mês` : "sem orçamento"}
                    {c.lgpd && " · LGPD"}
                    {c.cache && " · cache"}
                  </span>
                </button>
                {aberto === c.id && <Chaves cliente={c} />}
              </li>
            ))}
          </ul>
        )}
      </section>
      <NovoCliente />
    </div>
  );
}

function NovoCliente() {
  const qc = useQueryClient();
  const modelos = useQuery({ queryKey: ["modelos"], queryFn: () => api<Modelo[]>("/admin/modelos") });
  const [f, setF] = useState({ nome: "", rpm: "60", orcamento: "", cache: false, lgpd: true, modelos: [] as string[] });
  const criar = useMutation({
    mutationFn: () =>
      enviar("POST", "/admin/clientes", {
        nome: f.nome,
        rpm: Number(f.rpm) || 0,
        orcamento_micro: Math.round((Number(f.orcamento) || 0) * 1e6),
        cache: f.cache,
        lgpd: f.lgpd,
        modelos: f.modelos,
      }),
    onSuccess: () => {
      setF({ ...f, nome: "" });
      qc.invalidateQueries({ queryKey: ["clientes"] });
    },
  });
  const enviarForm = (e: FormEvent) => {
    e.preventDefault();
    criar.mutate();
  };
  return (
    <section aria-labelledby="novo">
      <h2 id="novo" className="mb-3 font-medium">Novo cliente</h2>
      <form onSubmit={enviarForm} className="space-y-3 text-sm">
        <label className="block">
          Nome da aplicação
          <input required value={f.nome} onChange={(e) => setF({ ...f, nome: e.target.value })} className={campo} />
        </label>
        <div className="grid grid-cols-2 gap-3">
          <label className="block">
            Requisições/min
            <input type="number" min="0" value={f.rpm} onChange={(e) => setF({ ...f, rpm: e.target.value })} className={campo} />
          </label>
          <label className="block">
            Orçamento (US$/mês)
            <input type="number" min="0" step="0.01" placeholder="sem limite" value={f.orcamento} onChange={(e) => setF({ ...f, orcamento: e.target.value })} className={campo} />
          </label>
        </div>
        <label className="flex items-center gap-2">
          <input type="checkbox" checked={f.lgpd} onChange={(e) => setF({ ...f, lgpd: e.target.checked })} />
          Mascarar dados pessoais (CPF, e-mail, telefone…)
        </label>
        <label className="flex items-center gap-2">
          <input type="checkbox" checked={f.cache} onChange={(e) => setF({ ...f, cache: e.target.checked })} />
          Reaproveitar respostas iguais (cache)
        </label>
        <fieldset>
          <legend>Modelos liberados <span className="text-suave">(nenhum marcado = todos)</span></legend>
          <div className="mt-1 flex flex-wrap gap-x-4">
            {modelos.data?.map((m) => (
              <label key={m.alias} className="flex items-center gap-1.5">
                <input
                  type="checkbox"
                  checked={f.modelos.includes(m.alias)}
                  onChange={(e) =>
                    setF({ ...f, modelos: e.target.checked ? [...f.modelos, m.alias] : f.modelos.filter((x) => x !== m.alias) })
                  }
                />
                {m.alias}
              </label>
            ))}
          </div>
        </fieldset>
        {criar.isError && <p role="alert" className="text-erro">{criar.error.message}</p>}
        <button disabled={criar.isPending} className="rounded-md bg-acento px-3 py-2 font-medium text-white disabled:opacity-60 dark:text-fundo">
          Cadastrar cliente
        </button>
      </form>
    </section>
  );
}

function Chaves({ cliente }: { cliente: Cliente }) {
  const qc = useQueryClient();
  const base = `/admin/clientes/${cliente.id}`;
  const chaves = useQuery({ queryKey: ["chaves", cliente.id], queryFn: () => api<Chave[]>(`${base}/chaves`) });
  const [nova, setNova] = useState("");
  const [copiada, setCopiada] = useState(false);
  const atualizar = () => qc.invalidateQueries({ queryKey: ["chaves", cliente.id] });
  const gerar = useMutation({
    mutationFn: () => api<{ chave: string }>(`${base}/chaves`, { method: "POST" }),
    onSuccess: (r) => {
      setNova(r.chave);
      setCopiada(false);
      atualizar();
    },
  });
  const revogar = useMutation({ mutationFn: (id: number) => enviar("DELETE", `${base}/chaves/${id}`), onSuccess: atualizar });
  const remover = useMutation({
    mutationFn: () => enviar("DELETE", base),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["clientes"] }),
  });

  return (
    <div className="mt-3 space-y-3 text-sm">
      {nova && (
        <div role="status" className="rounded-md border border-acento p-3">
          <p className="mb-2">Copie a chave agora. Ela não será mostrada de novo.</p>
          <div className="flex gap-2">
            <code className="min-w-0 flex-1 truncate rounded bg-fundo px-2 py-1">{nova}</code>
            <button
              className="rounded-md border border-borda px-2"
              onClick={async () => {
                await navigator.clipboard.writeText(nova);
                setCopiada(true);
              }}
            >
              {copiada ? "Copiada" : "Copiar"}
            </button>
          </div>
        </div>
      )}
      <ul className="space-y-1">
        {chaves.data?.map((k) => (
          <li key={k.id} className="flex items-center justify-between gap-3">
            <code className={k.revogada_em ? "text-suave line-through" : ""}>{k.prefixo}</code>
            {k.revogada_em ? (
              <span className="text-suave">revogada</span>
            ) : (
              <button
                className="text-erro hover:underline"
                onClick={() => confirm(`Revogar ${k.prefixo}? Aplicações usando esta chave param na hora.`) && revogar.mutate(k.id)}
              >
                Revogar
              </button>
            )}
          </li>
        ))}
      </ul>
      <div className="flex gap-4">
        <button className="text-acento hover:underline" onClick={() => gerar.mutate()}>
          Gerar chave
        </button>
        <button
          className="text-suave hover:text-erro"
          onClick={() => confirm(`Remover ${cliente.nome} e todas as chaves? O histórico de uso continua.`) && remover.mutate()}
        >
          Remover cliente
        </button>
      </div>
    </div>
  );
}
