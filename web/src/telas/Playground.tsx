import { useQuery } from "@tanstack/react-query";
import { useRef, useState, type FormEvent, type ReactNode } from "react";
import { api, type Modelo } from "../api";
import { botao, botaoSecundario, campo, cartao, Selo } from "../ui";

// The gateway returns in X-Comporta-Enviado (base64, UTF-8) the last message as it went out to the
// provider, already masked. atob yields "latin1" bytes; TextDecoder reassembles accented characters.
function decodificar(b64: string | null): string | null {
  if (!b64) return null;
  try {
    return new TextDecoder().decode(Uint8Array.from(atob(b64), (c) => c.charCodeAt(0)));
  } catch {
    return null;
  }
}

// Highlights the placeholders ([CPF_1], [EMAIL_2]...) that took the place of personal data.
function comMarcadores(texto: string): ReactNode[] {
  return texto.split(/(\[(?:CPF|CNPJ|CARTAO|EMAIL|TELEFONE)_\d+\])/).map((parte, i) =>
    i % 2 ? (
      <mark key={i} className="rounded bg-acento/15 px-1 font-mono text-[0.92em] font-medium text-acento">
        {parte}
      </mark>
    ) : (
      parte
    ),
  );
}

type Info = { status?: string; ttft?: number; total?: number; entrada?: number; saida?: number; requestId?: string };

// Playground calls the public API the way a client would: with the ck_ key pasted here (it stays
// only in the component's memory). fetch + ReadableStream because EventSource can't POST or
// send Authorization.
export default function Playground() {
  const modelos = useQuery({ queryKey: ["modelos"], queryFn: () => api<Modelo[]>("/admin/modelos") });
  const [chave, setChave] = useState("");
  const [modelo, setModelo] = useState("rapido");
  const [texto, setTexto] = useState("My CPF is 529.982.247-25 and my email is ana@example.com. Can you confirm my registration?");
  const [stream, setStream] = useState(true);
  const [cache, setCache] = useState(false);
  const [saida, setSaida] = useState("");
  const [enviado, setEnviado] = useState<string | null>(null);
  const [respondeu, setRespondeu] = useState(false);
  const [info, setInfo] = useState<Info>({});
  const [rodando, setRodando] = useState(false);
  const abortar = useRef<AbortController | null>(null);

  const enviar = async (e: FormEvent) => {
    e.preventDefault();
    abortar.current = new AbortController();
    setRodando(true);
    setSaida("");
    setEnviado(null);
    setRespondeu(false);
    setInfo({});
    const inicio = performance.now();
    let ttft = 0;
    try {
      const r = await fetch("/v1/chat/completions", {
        method: "POST",
        signal: abortar.current.signal,
        headers: {
          Authorization: `Bearer ${chave}`,
          "Content-Type": "application/json",
          ...(cache ? { "X-Comporta-Cache": "ligado" } : {}),
        },
        // include_usage: without it the gateway doesn't relay the usage chunk in the stream (like OpenAI).
        body: JSON.stringify({
          model: modelo,
          stream,
          ...(stream ? { stream_options: { include_usage: true } } : {}),
          messages: [{ role: "user", content: texto }],
        }),
      });
      setRespondeu(r.ok); // an error (invalid key, limit) says nothing about the key's LGPD setting
      setEnviado(decodificar(r.headers.get("X-Comporta-Enviado")));
      if (!r.ok) {
        const corpo = await r.json().catch(() => null);
        setInfo({ status: `${r.status}: ${corpo?.erro?.mensagem ?? r.statusText}` });
        return;
      }
      let uso: { prompt_tokens: number; completion_tokens: number } | undefined;
      if (!stream) {
        const v = await r.json();
        setSaida(v.choices?.[0]?.message?.content ?? "");
        uso = v.usage;
      } else {
        const leitor = r.body!.pipeThrough(new TextDecoderStream()).getReader();
        let buf = "";
        for (;;) {
          const { value, done } = await leitor.read();
          if (done) break;
          buf += value;
          const eventos = buf.split("\n\n");
          buf = eventos.pop()!;
          for (const ev of eventos) {
            const dado = ev.startsWith("data: ") ? ev.slice(6) : "";
            if (!dado || dado === "[DONE]") continue;
            const v = JSON.parse(dado);
            const pedaco = v.choices?.[0]?.delta?.content ?? "";
            if (pedaco && !ttft) ttft = performance.now() - inicio;
            setSaida((s) => s + pedaco);
            if (v.usage) uso = v.usage;
          }
        }
      }
      setInfo({
        ttft: stream && ttft ? Math.round(ttft) : undefined,
        total: Math.round(performance.now() - inicio),
        entrada: uso?.prompt_tokens,
        saida: uso?.completion_tokens,
        requestId: r.headers.get("X-Request-Id") ?? undefined,
      });
    } catch (err) {
      if ((err as Error).name !== "AbortError") setInfo({ status: String(err) });
    } finally {
      setRodando(false);
    }
  };

  return (
    <div className="grid items-start gap-6 2xl:grid-cols-[22rem_1fr]">
      <form onSubmit={enviar} className={`${cartao} space-y-4 p-5 text-sm`}>
        <label className="block font-medium">
          Client key
          <input
            required
            type="password"
            placeholder="ck_…"
            autoComplete="off"
            value={chave}
            onChange={(e) => setChave(e.target.value)}
            className={`${campo} font-mono font-normal`}
          />
        </label>
        <label className="block font-medium">
          Model
          <select value={modelo} onChange={(e) => setModelo(e.target.value)} className={`${campo} font-normal`}>
            {modelos.data?.map((m) => (
              <option key={m.alias} value={m.alias}>
                {m.alias} ({m.destinos[0]}){m.shadow ? " with shadow" : ""}
              </option>
            ))}
          </select>
        </label>
        <label className="block font-medium">
          Message
          <textarea rows={5} value={texto} onChange={(e) => setTexto(e.target.value)} className={`${campo} font-normal`} />
        </label>
        <div className="flex flex-wrap gap-x-6 gap-y-2">
          <label className="flex items-center gap-2">
            <input type="checkbox" checked={stream} onChange={(e) => setStream(e.target.checked)} /> Stream
          </label>
          <label className="flex items-center gap-2">
            <input type="checkbox" checked={cache} onChange={(e) => setCache(e.target.checked)} /> Use cache
          </label>
        </div>
        <div className="flex gap-3">
          <button disabled={rodando} className={`${botao} flex-1`}>
            {rodando ? "Sending…" : "Send"}
          </button>
          {rodando && (
            <button type="button" className={botaoSecundario} onClick={() => abortar.current?.abort()}>
              Cancel
            </button>
          )}
        </div>
      </form>

      <div className="space-y-4" aria-live="polite">
        <div className="grid gap-4 lg:grid-cols-2">
          <Painel
            titulo="What the provider received"
            selo={enviado !== null ? <Selo tom="ok">data masked</Selo> : respondeu ? <Selo tom="alerta">LGPD off for this key</Selo> : null}
          >
            {enviado !== null ? (
              comMarcadores(enviado)
            ) : (
              <span className="text-suave">
                {respondeu
                  ? "This client doesn't mask data: the message went out as written. Turn LGPD on in Clients and keys."
                  : "This shows the message as it went out to the AI, with CPF, email and phone replaced by placeholders."}
              </span>
            )}
          </Painel>
          <Painel titulo="What came back to you" selo={enviado !== null && saida ? <Selo tom="ok">data restored</Selo> : null}>
            {saida || (
              <span className="text-suave">{rodando ? "Waiting for the first token…" : "The answer shows up here, token by token with streaming on."}</span>
            )}
          </Painel>
        </div>
        <Metricas info={info} />
      </div>
    </div>
  );
}

function Painel({ titulo, selo, children }: { titulo: string; selo: ReactNode; children: ReactNode }) {
  return (
    <section className={`${cartao} flex flex-col`}>
      <header className="flex min-h-12 flex-wrap items-center justify-between gap-2 border-b border-borda px-4 py-2.5">
        <h2 className="whitespace-nowrap text-sm font-medium">{titulo}</h2>
        {selo}
      </header>
      <div className="min-h-36 flex-1 whitespace-pre-wrap px-4 py-3 leading-relaxed">{children}</div>
    </section>
  );
}

function Metricas({ info }: { info: Info }) {
  if (info.status)
    return (
      <p role="alert" className="rounded-md border border-erro/30 bg-erro/8 px-3 py-2 text-sm text-erro">
        {info.status}
      </p>
    );
  const itens = [
    info.ttft !== undefined && ["First token", `${info.ttft} ms`],
    info.total !== undefined && ["Total", `${info.total} ms`],
    info.entrada !== undefined && ["Tokens", `${info.entrada} in · ${info.saida} out`],
    info.requestId && ["Request", info.requestId],
  ].filter(Boolean) as [string, string][];
  if (!itens.length) return null;
  return (
    <dl className="flex flex-wrap gap-2 text-xs">
      {itens.map(([rotulo, valor]) => (
        <div key={rotulo} className="flex gap-1.5 rounded-md border border-borda bg-superficie px-2.5 py-1">
          <dt className="text-suave">{rotulo}</dt>
          <dd className="font-medium">{valor}</dd>
        </div>
      ))}
    </dl>
  );
}
