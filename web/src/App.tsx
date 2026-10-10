import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState, type FormEvent } from "react";
import { api, ErroApi, enviar } from "./api";
import Clientes from "./telas/Clientes";
import Playground from "./telas/Playground";
import Shadow from "./telas/Shadow";
import Visao from "./telas/Visao";

const telas = {
  visao: { nome: "Visão geral", el: Visao },
  clientes: { nome: "Clientes e chaves", el: Clientes },
  shadow: { nome: "Migração (shadow)", el: Shadow },
  playground: { nome: "Playground", el: Playground },
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
  const qc = useQueryClient();

  if (sessao.isPending) return null;
  if (sessao.isError) return <Login />;

  const Atual = telas[tela].el;
  return (
    <div className="min-h-screen md:grid md:grid-cols-[13rem_1fr]">
      <nav className="border-b border-borda px-4 py-4 md:min-h-screen md:border-r md:border-b-0" aria-label="Seções">
        <p className="mb-6 text-lg font-semibold tracking-tight">Comporta</p>
        <ul className="flex flex-wrap gap-1 md:flex-col">
          {(Object.keys(telas) as Tela[]).map((t) => (
            <li key={t}>
              <a
                href={`#${t}`}
                aria-current={t === tela ? "page" : undefined}
                className="block rounded-md px-3 py-1.5 text-sm text-suave hover:text-texto aria-[current=page]:bg-superficie aria-[current=page]:font-medium aria-[current=page]:text-texto"
              >
                {telas[t].nome}
              </a>
            </li>
          ))}
        </ul>
        <button
          className="mt-6 px-3 text-sm text-suave hover:text-texto"
          onClick={async () => {
            await enviar("POST", "/admin/logout");
            qc.resetQueries({ queryKey: ["sessao"] });
          }}
        >
          Sair
        </button>
      </nav>
      <main className="mx-auto w-full max-w-6xl px-4 py-6 md:px-8">
        <h1 className="mb-6 text-2xl font-semibold tracking-tight">{telas[tela].nome}</h1>
        <Atual />
      </main>
    </div>
  );
}

function Login() {
  const qc = useQueryClient();
  const [token, setToken] = useState("");
  const [erro, setErro] = useState("");
  const entrar = async (e: FormEvent) => {
    e.preventDefault();
    try {
      await enviar("POST", "/admin/login", { token });
      qc.invalidateQueries({ queryKey: ["sessao"] });
    } catch (err) {
      setErro(err instanceof ErroApi && err.status === 401 ? "Token incorreto. Confira o ADMIN_TOKEN do servidor." : String(err));
    }
  };
  return (
    <main className="grid min-h-screen place-items-center px-4">
      <form onSubmit={entrar} className="w-full max-w-sm space-y-4">
        <div>
          <p className="text-2xl font-semibold tracking-tight">Comporta</p>
          <p className="text-sm text-suave">Controle de acesso, custo e dados pessoais nas chamadas de IA.</p>
        </div>
        <label className="block text-sm">
          Token de admin
          <input
            type="password"
            autoFocus
            value={token}
            onChange={(e) => setToken(e.target.value)}
            className="mt-1 block w-full rounded-md border border-borda bg-superficie px-3 py-2"
          />
        </label>
        {erro && <p role="alert" className="text-sm text-erro">{erro}</p>}
        <button className="w-full rounded-md bg-acento px-3 py-2 font-medium text-white dark:text-fundo">Entrar</button>
      </form>
    </main>
  );
}
