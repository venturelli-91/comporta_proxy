import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { api, enviar, usd, type Chave, type Cliente, type Modelo } from "../api";
import { botao, botaoSecundario, botaoTexto, campo, cartao, IconeCopiar, IconeSeta, Selo } from "../ui";
import { Estado } from "./Estado";

export default function Clientes() {
  const clientes = useQuery({ queryKey: ["clientes"], queryFn: () => api<Cliente[]>("/admin/clientes") });
  const [aberto, setAberto] = useState<number | null>(null);
  if (!clientes.data) return <Estado q={clientes} />;
  return (
    <div className="grid items-start gap-10 lg:grid-cols-[1fr_22rem]">
      <section aria-labelledby="lista">
        <h2 id="lista" className="mb-3 font-medium">
          Clientes <span className="font-normal text-suave">({clientes.data.length})</span>
        </h2>
        {clientes.data.length === 0 ? (
          <p className={`${cartao} p-5 text-sm text-suave`}>
            Nenhum cliente ainda. Cadastre o primeiro ao lado: cada aplicação recebe a própria chave <code className="font-mono">ck_</code>, com
            limite e orçamento.
          </p>
        ) : (
          <ul className={`${cartao} divide-y divide-borda overflow-hidden`}>
            {clientes.data.map((c) => (
              <li key={c.id}>
                <button
                  className="flex w-full items-center gap-3 px-4 py-3 text-left hover:bg-realce"
                  aria-expanded={aberto === c.id}
                  onClick={() => setAberto(aberto === c.id ? null : c.id)}
                >
                  <IconeSeta aberto={aberto === c.id} />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate font-medium">{c.nome}</span>
                    <span className="block text-sm text-suave">
                      {c.rpm ? `${c.rpm} req/min` : "sem limite"} · {c.orcamento_micro ? `${usd(c.orcamento_micro)}/mês` : "sem orçamento"}
                    </span>
                  </span>
                  <span className="flex flex-wrap justify-end gap-1.5">
                    {c.lgpd && <Selo tom="ok">LGPD</Selo>}
                    {c.cache && <Selo tom="neutro">cache</Selo>}
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
    <section aria-labelledby="novo" className={`${cartao} p-5 lg:sticky lg:top-6`}>
      <h2 id="novo" className="mb-4 font-medium">
        Novo cliente
      </h2>
      <form onSubmit={enviarForm} className="space-y-4 text-sm">
        <label className="block font-medium">
          Nome da aplicação
          <input required value={f.nome} onChange={(e) => setF({ ...f, nome: e.target.value })} placeholder="ex.: app-atendimento" className={`${campo} font-normal`} />
        </label>
        <div className="grid grid-cols-2 gap-3">
          <label className="block font-medium">
            Requisições/min
            <input type="number" min="0" value={f.rpm} onChange={(e) => setF({ ...f, rpm: e.target.value })} className={`${campo} font-normal`} />
          </label>
          <label className="block font-medium">
            Orçamento (US$/mês)
            <input
              type="number"
              min="0"
              step="0.01"
              placeholder="sem limite"
              value={f.orcamento}
              onChange={(e) => setF({ ...f, orcamento: e.target.value })}
              className={`${campo} font-normal`}
            />
          </label>
        </div>
        <div className="space-y-2">
          <label className="flex items-start gap-2">
            <input type="checkbox" className="mt-0.5" checked={f.lgpd} onChange={(e) => setF({ ...f, lgpd: e.target.checked })} />
            <span>
              Mascarar dados pessoais
              <span className="block text-suave">CPF, CNPJ, e-mail, telefone e cartão nunca saem para o provedor.</span>
            </span>
          </label>
          <label className="flex items-start gap-2">
            <input type="checkbox" className="mt-0.5" checked={f.cache} onChange={(e) => setF({ ...f, cache: e.target.checked })} />
            <span>
              Reaproveitar respostas iguais
              <span className="block text-suave">Pedidos idênticos respondem do cache, sem custo.</span>
            </span>
          </label>
        </div>
        <fieldset>
          <legend className="font-medium">
            Modelos liberados <span className="font-normal text-suave">(nenhum marcado = todos)</span>
          </legend>
          <div className="mt-2 flex flex-wrap gap-2">
            {modelos.data?.map((m) => (
              <label
                key={m.alias}
                className="flex items-center gap-1.5 rounded-md border border-borda px-2.5 py-1 hover:bg-realce has-checked:border-acento has-checked:bg-acento/8"
              >
                <input
                  type="checkbox"
                  checked={f.modelos.includes(m.alias)}
                  onChange={(e) => setF({ ...f, modelos: e.target.checked ? [...f.modelos, m.alias] : f.modelos.filter((x) => x !== m.alias) })}
                />
                {m.alias}
              </label>
            ))}
          </div>
        </fieldset>
        {criar.isError && (
          <p role="alert" className="text-erro">
            {criar.error.message}
          </p>
        )}
        <button disabled={criar.isPending} className={`${botao} w-full`}>
          {criar.isPending ? "Cadastrando…" : "Cadastrar cliente"}
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
    <div className="space-y-4 border-t border-borda bg-realce/60 px-4 py-4 pl-11 text-sm">
      {nova && (
        <div role="status" className="rounded-md border border-acento/40 bg-acento/8 p-3">
          <p className="mb-2 font-medium">Copie a chave agora: ela não será mostrada de novo.</p>
          <div className="flex gap-2">
            <code className="min-w-0 flex-1 truncate rounded border border-borda bg-superficie px-2 py-1.5 font-mono">{nova}</code>
            <button
              className={botaoSecundario}
              onClick={async () => {
                await navigator.clipboard.writeText(nova);
                setCopiada(true);
              }}
            >
              <IconeCopiar />
              {copiada ? "Copiada" : "Copiar"}
            </button>
          </div>
        </div>
      )}
      <div>
        <h3 className="mb-2 text-suave">Chaves</h3>
        {chaves.data?.length === 0 && <p className="text-suave">Nenhuma chave. Gere uma para este cliente chamar a API.</p>}
        <ul className="space-y-1.5">
          {chaves.data?.map((k) => (
            <li key={k.id} className="flex items-center justify-between gap-3">
              <code className={`font-mono ${k.revogada_em ? "text-suave line-through" : ""}`}>{k.prefixo}</code>
              {k.revogada_em ? (
                <Selo tom="neutro">revogada</Selo>
              ) : (
                <button
                  className={`${botaoTexto} text-erro`}
                  onClick={() => confirm(`Revogar ${k.prefixo}? Aplicações usando esta chave param na hora.`) && revogar.mutate(k.id)}
                >
                  Revogar
                </button>
              )}
            </li>
          ))}
        </ul>
      </div>
      <div className="flex flex-wrap items-center gap-4">
        <button className={botaoSecundario} disabled={gerar.isPending} onClick={() => gerar.mutate()}>
          {gerar.isPending ? "Gerando…" : "Gerar chave"}
        </button>
        <button
          className={`${botaoTexto} text-suave hover:text-erro`}
          onClick={() => confirm(`Remover ${cliente.nome} e todas as chaves? O histórico de uso continua.`) && remover.mutate()}
        >
          Remover cliente
        </button>
      </div>
    </div>
  );
}
