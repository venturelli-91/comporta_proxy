import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState, type FormEvent } from "react";
import { api, ErroApi, enviar } from "./api";
import Clientes from "./telas/Clientes";
import { Estado } from "./telas/Estado";
import Playground from "./telas/Playground";
import Shadow from "./telas/Shadow";
import Visao from "./telas/Visao";
import { botao, campo, cartao, IconeClientes, IconePlayground, IconeSair, IconeShadow, IconeVisao, Marca, SeletorTema } from "./ui";

const telas = {
  visao: { nome: "Overview", curto: "Overview", el: Visao, icone: IconeVisao },
  clientes: { nome: "Clients and keys", curto: "Clients", el: Clientes, icone: IconeClientes },
  shadow: { nome: "Migration (shadow)", curto: "Shadow", el: Shadow, icone: IconeShadow },
  playground: { nome: "Playground", curto: "Playground", el: Playground, icone: IconePlayground },
} as const;
type Tela = keyof typeof telas;

const daHash = (): Tela => {
  const h = location.hash.slice(1);
  return h in telas ? (h as Tela) : "visao";
};

export default function App() {
  // TanStack Query v5 doesn't accept undefined as data; the session's 204 becomes true.
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
  if (sessao.isPending) return null;
  // Only a 401 means "no session"; a server that is down or a 500 shows the error, not a login that wouldn't help.
  if (sessao.error instanceof ErroApi && sessao.error.status === 401) return <Login />;
  if (sessao.isError)
    return (
      <main className="grid min-h-screen place-items-center px-4">
        <Estado q={sessao} />
      </main>
    );

  const Atual = telas[tela].el;
  return (
    <div className="min-h-screen md:grid md:grid-cols-[15rem_1fr]">
      {/* Mobile: top bar with brand, theme and sign out; the sections go to the tab bar at the bottom. */}
      <header className="sticky top-0 z-20 flex items-center justify-between gap-2 border-b border-borda bg-lateral/95 px-4 py-2.5 backdrop-blur md:hidden">
        <Marca />
        <div className="flex items-center gap-1">
          <SeletorTema />
          <BotaoSair rotulo={false} />
        </div>
      </header>
      <nav
        className="fixed inset-x-0 bottom-0 z-20 border-t border-borda bg-lateral/95 pb-[env(safe-area-inset-bottom)] backdrop-blur md:sticky md:top-0 md:flex md:h-screen md:flex-col md:gap-4 md:border-t-0 md:border-r md:bg-lateral md:px-4 md:py-6 md:backdrop-blur-none"
        aria-label="Sections"
      >
        <div className="hidden px-2 md:block">
          <Marca />
        </div>
        <ul className="grid grid-cols-4 md:flex md:flex-col md:gap-1">
          {(Object.keys(telas) as Tela[]).map((t) => {
            const I = telas[t].icone;
            return (
              <li key={t}>
                <a
                  href={`#${t}`}
                  aria-current={t === tela ? "page" : undefined}
                  className="flex flex-col items-center gap-1 px-1 py-2 text-[11px] text-suave hover:text-texto aria-[current=page]:font-medium aria-[current=page]:text-acento md:flex-row md:gap-2.5 md:rounded-md md:px-2.5 md:py-2 md:text-sm md:hover:bg-realce md:aria-[current=page]:bg-superficie md:aria-[current=page]:text-texto md:aria-[current=page]:shadow-cartao md:aria-[current=page]:[&>svg]:text-acento"
                >
                  <I />
                  <span className="md:hidden">{telas[t].curto}</span>
                  <span className="hidden md:inline">{telas[t].nome}</span>
                </a>
              </li>
            );
          })}
        </ul>
        <div className="mt-auto hidden items-center justify-between gap-2 px-2 md:flex">
          <SeletorTema />
          <BotaoSair rotulo />
        </div>
      </nav>
      <main className="mx-auto w-full max-w-6xl min-w-0 px-4 pt-6 pb-28 md:px-10 md:py-10">
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
      if (err instanceof ErroApi && err.status === 401) setErro("Wrong token. Check the server's ADMIN_TOKEN.");
      else if (err instanceof ErroApi && err.status === 429) setErro("Too many attempts. Wait a minute and try again.");
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
            <h1 className="text-lg font-semibold tracking-tight">Sign in to the dashboard</h1>
            <p className="mt-1 text-sm text-suave">Access, cost and personal data in your AI calls, in one place.</p>
          </div>
          <label className="block text-sm font-medium">
            Admin token
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
            {entrando ? "Signing in…" : "Sign in"}
          </button>
        </form>
      </div>
    </main>
  );
}

function BotaoSair({ rotulo }: { rotulo: boolean }) {
  const qc = useQueryClient();
  return (
    <button
      aria-label="Sign out"
      title="Sign out"
      className="inline-flex items-center gap-1.5 rounded-md px-2 py-1.5 text-sm text-suave hover:bg-realce hover:text-texto"
      onClick={async () => {
        await enviar("POST", "/admin/logout");
        qc.resetQueries({ queryKey: ["sessao"] });
      }}
    >
      <IconeSair />
      {rotulo && "Sign out"}
    </button>
  );
}
