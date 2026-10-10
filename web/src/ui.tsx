import { useState, type ReactNode, type SVGProps } from "react";

// Vocabulário visual único do painel: o mesmo botão, campo e cartão em todas as telas.
export const campo =
  "mt-1 block w-full rounded-md border border-borda bg-superficie px-3 py-2 text-texto hover:border-suave/60 disabled:opacity-60";
export const botao =
  "inline-flex items-center justify-center gap-2 rounded-md bg-acento px-3.5 py-2 text-sm font-medium text-sobre-acento shadow-cartao hover:brightness-110 active:brightness-95 disabled:opacity-60 disabled:hover:brightness-100";
export const botaoSecundario =
  "inline-flex items-center justify-center gap-2 rounded-md border border-borda bg-superficie px-3 py-2 text-sm font-medium text-texto hover:bg-realce active:bg-borda/60";
export const botaoTexto = "inline-flex items-center gap-1.5 rounded text-sm font-medium hover:underline underline-offset-4";
export const cartao = "rounded-lg border border-borda bg-superficie shadow-cartao";

// Selo de estado: cor + rótulo (o estado nunca depende só da cor).
const tons = {
  ok: "bg-acento/12 text-acento ring-acento/25",
  alerta: "bg-alerta/12 text-alerta ring-alerta/25",
  erro: "bg-erro/12 text-erro ring-erro/25",
  neutro: "bg-borda/60 text-suave ring-borda",
} as const;
export function Selo({ tom, children }: { tom: keyof typeof tons; children: ReactNode }) {
  return (
    <span className={`inline-flex shrink-0 items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset ${tons[tom]}`}>
      {tom !== "neutro" && <span className="size-1.5 rounded-full bg-current" aria-hidden />}
      {children}
    </span>
  );
}

// Esqueleto ocupa o lugar do conteúdo enquanto carrega: o layout não pula quando os dados chegam.
export function Esqueleto({ className = "" }: { className?: string }) {
  return <div className={`animate-pulso rounded-md bg-borda/70 ${className}`} aria-hidden />;
}

// Ícones: traço de 1,75 px em 20×20, herdam a cor do texto. SVG inline para não puxar dependência.
function Icone({ children, ...p }: SVGProps<SVGSVGElement>) {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth={1.75} strokeLinecap="round" strokeLinejoin="round" className="size-4.5 shrink-0" aria-hidden {...p}>
      {children}
    </svg>
  );
}
export const IconeVisao = () => (
  <Icone>
    <path d="M3 17V9M8 17V4M13 17v-6M18 17V7" />
  </Icone>
);
export const IconeClientes = () => (
  <Icone>
    <circle cx="7" cy="7.5" r="3" />
    <path d="M2 17c.6-2.6 2.6-4 5-4s4.4 1.4 5 4M13 4.8a3 3 0 0 1 0 5.4M15 13.2c1.6.5 2.6 1.8 3 3.8" />
  </Icone>
);
export const IconeShadow = () => (
  <Icone>
    <path d="M3 6h9M9 3l3 3-3 3M17 14H8M11 11l-3 3 3 3" />
  </Icone>
);
export const IconePlayground = () => (
  <Icone>
    <path d="M4 5l4 5-4 5M10 15h6" />
  </Icone>
);
export const IconeCopiar = () => (
  <Icone>
    <rect x="7" y="7" width="10" height="10" rx="2" />
    <path d="M13 7V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v6a2 2 0 0 0 2 2h2" />
  </Icone>
);
export const IconeSeta = ({ aberto }: { aberto: boolean }) => (
  <Icone className={`size-4 shrink-0 text-suave transition-transform duration-200 ${aberto ? "rotate-90" : ""}`}>
    <path d="M8 5l5 5-5 5" />
  </Icone>
);
export const IconeSair = () => (
  <Icone>
    <path d="M8 4H5a1 1 0 0 0-1 1v10a1 1 0 0 0 1 1h3M12 6l4 4-4 4M16 10H8" />
  </Icone>
);
const IconeSol = () => (
  <Icone>
    <circle cx="10" cy="10" r="3.5" />
    <path d="M10 2v1.5M10 16.5V18M2 10h1.5M16.5 10H18M4.3 4.3l1.1 1.1M14.6 14.6l1.1 1.1M4.3 15.7l1.1-1.1M14.6 5.4l1.1-1.1" />
  </Icone>
);
const IconeLua = () => (
  <Icone>
    <path d="M16.5 12.5A7 7 0 0 1 7.5 3.5a7 7 0 1 0 9 9Z" />
  </Icone>
);
const IconeMonitor = () => (
  <Icone>
    <rect x="2.5" y="3.5" width="15" height="10" rx="1.5" />
    <path d="M7 17h6M10 13.5V17" />
  </Icone>
);

// Marca: a comporta (duas colunas e duas pranchas), a mesma do favicon.
export function Marca() {
  return (
    <span className="inline-flex items-center gap-2.5 text-base font-semibold tracking-tight">
      <svg viewBox="0 0 32 32" className="size-7" aria-hidden>
        <rect width="32" height="32" rx="8" fill="var(--acento)" />
        <path d="M9 8v16M23 8v16M9 13h14M9 19h14" stroke="var(--sobre-acento)" strokeWidth="2.5" strokeLinecap="round" />
      </svg>
      Comporta
    </span>
  );
}

// Tema: sistema por padrão; a escolha manual fica no localStorage (lido também no index.html
// antes da pintura). localStorage pode falhar (aba privada, bloqueio): aí só não lembra.
type Tema = "claro" | "escuro" | "sistema";
const opcoes: { tema: Tema; nome: string; icone: () => ReactNode }[] = [
  { tema: "claro", nome: "Tema claro", icone: IconeSol },
  { tema: "escuro", nome: "Tema escuro", icone: IconeLua },
  { tema: "sistema", nome: "Tema do sistema", icone: IconeMonitor },
];
function lerTema(): Tema {
  try {
    const t = localStorage.getItem("tema");
    return t === "claro" || t === "escuro" ? t : "sistema";
  } catch {
    return "sistema";
  }
}
export function SeletorTema() {
  const [tema, setTema] = useState<Tema>(lerTema);
  const escolher = (t: Tema) => {
    setTema(t);
    if (t === "sistema") delete document.documentElement.dataset.tema;
    else document.documentElement.dataset.tema = t;
    try {
      if (t === "sistema") localStorage.removeItem("tema");
      else localStorage.setItem("tema", t);
    } catch {
      /* sem armazenamento: o tema vale só nesta visita */
    }
  };
  return (
    <div role="radiogroup" aria-label="Tema" className="inline-flex rounded-md border border-borda bg-superficie p-0.5">
      {opcoes.map(({ tema: t, nome, icone: I }) => (
        <button
          key={t}
          role="radio"
          aria-checked={tema === t}
          aria-label={nome}
          title={nome}
          onClick={() => escolher(t)}
          className="rounded px-2 py-1.5 text-suave hover:text-texto aria-checked:bg-realce aria-checked:text-acento"
        >
          <I />
        </button>
      ))}
    </div>
  );
}
