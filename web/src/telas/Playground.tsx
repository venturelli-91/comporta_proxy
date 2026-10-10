import { useQuery } from "@tanstack/react-query";
import { useRef, useState, type FormEvent } from "react";
import { api, type Modelo } from "../api";

const campo = "mt-1 block w-full rounded-md border border-borda bg-superficie px-3 py-2";

// Playground chama a API pública como um cliente faria: com a chave ck_ colada aqui (que fica
// só na memória do componente). fetch + ReadableStream porque EventSource não faz POST nem
// manda Authorization.
export default function Playground() {
  const modelos = useQuery({ queryKey: ["modelos"], queryFn: () => api<Modelo[]>("/admin/modelos") });
  const [chave, setChave] = useState("");
  const [modelo, setModelo] = useState("rapido");
  const [texto, setTexto] = useState("Meu CPF é 529.982.247-25. Pode confirmar o cadastro?");
  const [stream, setStream] = useState(true);
  const [cache, setCache] = useState(false);
  const [saida, setSaida] = useState("");
  const [info, setInfo] = useState("");
  const [rodando, setRodando] = useState(false);
  const abortar = useRef<AbortController | null>(null);

  const enviar = async (e: FormEvent) => {
    e.preventDefault();
    abortar.current = new AbortController();
    setRodando(true);
    setSaida("");
    setInfo("");
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
        body: JSON.stringify({ model: modelo, stream, messages: [{ role: "user", content: texto }] }),
      });
      if (!r.ok) {
        const corpo = await r.json().catch(() => null);
        setInfo(`${r.status}: ${corpo?.erro?.mensagem ?? r.statusText}`);
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
      const total = Math.round(performance.now() - inicio);
      setInfo(
        [
          stream && ttft ? `primeiro token em ${Math.round(ttft)} ms` : "",
          `total ${total} ms`,
          uso ? `${uso.prompt_tokens} tokens de entrada, ${uso.completion_tokens} de saída` : "",
          r.headers.get("X-Request-Id") ? `request ${r.headers.get("X-Request-Id")}` : "",
        ]
          .filter(Boolean)
          .join(" · "),
      );
    } catch (err) {
      if ((err as Error).name !== "AbortError") setInfo(String(err));
    } finally {
      setRodando(false);
    }
  };

  return (
    <div className="grid gap-8 lg:grid-cols-2">
      <form onSubmit={enviar} className="space-y-3 text-sm">
        <label className="block">
          Chave do cliente
          <input
            required
            type="password"
            placeholder="ck_…"
            autoComplete="off"
            value={chave}
            onChange={(e) => setChave(e.target.value)}
            className={campo}
          />
        </label>
        <label className="block">
          Modelo
          <select value={modelo} onChange={(e) => setModelo(e.target.value)} className={campo}>
            {modelos.data?.map((m) => (
              <option key={m.alias} value={m.alias}>
                {m.alias} ({m.destinos[0]}){m.shadow ? " com shadow" : ""}
              </option>
            ))}
          </select>
        </label>
        <label className="block">
          Mensagem
          <textarea rows={5} value={texto} onChange={(e) => setTexto(e.target.value)} className={campo} />
        </label>
        <div className="flex flex-wrap gap-x-6 gap-y-2">
          <label className="flex items-center gap-2">
            <input type="checkbox" checked={stream} onChange={(e) => setStream(e.target.checked)} /> Stream
          </label>
          <label className="flex items-center gap-2">
            <input type="checkbox" checked={cache} onChange={(e) => setCache(e.target.checked)} /> Usar cache
          </label>
        </div>
        <div className="flex gap-3">
          <button disabled={rodando} className="rounded-md bg-acento px-3 py-2 font-medium text-sobre-acento disabled:opacity-60 ">
            Enviar
          </button>
          {rodando && (
            <button type="button" className="rounded-md border border-borda px-3 py-2" onClick={() => abortar.current?.abort()}>
              Cancelar
            </button>
          )}
        </div>
      </form>
      <section aria-live="polite" aria-labelledby="resp">
        <h2 id="resp" className="mb-2 font-medium">Resposta</h2>
        <div className="min-h-40 whitespace-pre-wrap rounded-lg border border-borda bg-superficie p-4">
          {saida || <span className="text-suave">A resposta aparece aqui, token a token quando o stream estiver ligado.</span>}
        </div>
        {info && <p className="mt-2 text-sm text-suave">{info}</p>}
      </section>
    </div>
  );
}
