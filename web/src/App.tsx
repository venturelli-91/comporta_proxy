import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState, type FormEvent } from "react";
import { api, ErroApi, enviar } from "./api";
import Clientes from "./telas/Clientes";
import Playground from "./telas/Playground";
import Shadow from "./telas/Shadow";
import Visao from "./telas/Visao";
import { botao, campo, cartao, IconeClientes, IconePlayground, IconeSair, IconeShadow, IconeVisao, Marca, SeletorTema } from "./ui";

const telas = {
  visao: { nome: "Visão geral", el: Visao, icone: IconeVisao },
  clientes: { nome: "Clientes e chaves", el: Clientes, icone: IconeClientes },
  shadow: { nome: "Migração (shadow)", el: Shadow, icone: IconeShadow },
  playground: { nome: "Playground", el: Playground, icone: IconePlayground },
} as const;
type Tela = keyof typeof telas;

const daHash = (): Tela => {
  const h = location.hash.slice(1);
  return h in telas ? (h as Tela) : "visao";
};

export default function App() {
  // O v5 do TanStack Query não aceita undefined como dado; o 204 da sessão vira true.
  const sessao = useQuery({ queryKey: ["sessao"], queryFn: () => api<void>("/admin/sessao").then(() => true) });
  const [tela, setTela] = useState<Tela>(daHash);
  useEffect(() => {
    const f = () => setTela(daHash());
    addEventListener("hashchange", f);
    return () => removeEventListener("hashchange", f);
  }, []);
  useEffect(() => {
    document.title = `${telas[tela].nome} · Comporta`;
  }, [tela]);
  const qc = useQueryClient();

  if (sessao.isPending) return null;
  if (sessao.isError) return <Login />;

  const Atual = telas[tela].el;
  return (
    <div className="min-h-screen md:grid md:grid-cols-[15rem_1fr]">
      <nav
        className="flex flex-col gap-4 border-b border-borda bg-lateral px-4 py-4 md:sticky md:top-0 md:h-screen md:border-r md:border-b-0 md:py-6"
        aria-label="Seções"
      >
        <div className="flex items-center justify-between px-2">
          <Marca />
        </div>
        <ul className="flex gap-1 overflow-x-auto md:flex-col md:overflow-visible">
          {(Object.keys(telas) as Tela[]).map((t) => {
            const I = telas[t].icone;
            return (
              <li key={t} className="shrink-0">
                <a
                  href={`#${t}`}
                  aria-current={t === tela ? "page" : undefined}
                  className="flex items-center gap-2.5 rounded-md px-2.5 py-2 text-sm text-suave hover:bg-realce hover:text-texto aria-[current=page]:bg-superficie aria-[current=page]:font-medium aria-[current=page]:text-texto aria-[current=page]:shadow-cartao aria-[current=page]:[&>svg]:text-acento"
                >
                  <I />
                  {telas[t].nome}
                </a>
              </li>
            );
          })}
        </ul>
        <div className="flex items-center justify-between gap-2 px-2 md:mt-auto">
          <SeletorTema />
          <button
            className="inline-flex items-center gap-1.5 rounded-md px-2 py-1.5 text-sm text-suave hover:bg-realce hover:text-texto"
            onClick={async () => {
              await enviar("POST", "/admin/logout");
              qc.resetQueries({ queryKey: ["sessao"] });
            }}
          >
            <IconeSair />
            Sair
          </button>
        </div>
      </nav>
      <main className="mx-auto w-full max-w-6xl px-4 py-6 md:px-10 md:py-10">
        <h1 className="mb-8 text-2xl font-semibold tracking-tight">{telas[tela].nome}</h1>
        <Atual />
      </main>
    </div>
  );
}

function Login() {
  const qc = useQueryClient();
  const [token, setToken] = useState("");
  const [erro, setErro] = useState("");
  const [entrando, setEntrando] = useState(false);
  const entrar = async (e: FormEvent) => {
    e.preventDefault();
    setEntrando(true);
    setErro("");
    try {
      await enviar("POST", "/admin/login", { token });
      qc.invalidateQueries({ queryKey: ["sessao"] });
    } catch (err) {
      if (err instanceof ErroApi && err.status === 401) setErro("Token incorreto. Confira o ADMIN_TOKEN do servidor.");
      else if (err instanceof ErroApi && err.status === 429) setErro("Muitas tentativas. Aguarde um minuto e tente de novo.");
      else setErro(String(err));
    } finally {
      setEntrando(false);
    }
  };
  return (
    <main className="grid min-h-screen place-items-center bg-lateral px-4">
      <div className="w-full max-w-sm">
        <div className="mb-6 flex justify-between">
          <Marca />
          <SeletorTema />
        </div>
        <form onSubmit={entrar} className={`${cartao} space-y-5 p-6`}>
          <div>
            <h1 className="text-lg font-semibold tracking-tight">Entrar no painel</h1>
            <p className="mt-1 text-sm text-suave">Acesso, custo e dados pessoais nas chamadas de IA, num lugar só.</p>
          </div>
          <label className="block text-sm font-medium">
            Token de admin
            <input
              type="password"
              autoFocus
              required
              autoComplete="current-password"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              aria-invalid={erro ? true : undefined}
              aria-describedby={erro ? "erro-login" : undefined}
              className={`${campo} font-normal`}
            />
          </label>
          {erro && (
            <p id="erro-login" role="alert" className="text-sm text-erro">
              {erro}
            </p>
          )}
          <button disabled={entrando} className={`${botao} w-full`}>
            {entrando ? "Entrando…" : "Entrar"}
          </button>
        </form>
      </div>
    </main>
  );
}
