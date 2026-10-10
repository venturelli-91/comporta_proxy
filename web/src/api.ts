// Contrato da API do admin: espelha internal/httpapi/admin.go e internal/armazenamento.

export type Totais = {
  requisicoes: number;
  erros: number;
  cache_hits: number;
  tokens: number;
  custo_micro: number;
  economia_micro: number;
  pii: number;
  p95_ms: number;
};
export type Grupo = Totais & { nome: string };
export type Comparacao = {
  modelo: string;
  destino: string;
  shadow: boolean;
  requisicoes: number;
  latencia_media_ms: number;
  custo_medio_micro: number;
  tamanho_medio: number;
};
export type Relatorio = {
  total: Totais;
  por_cliente: Grupo[];
  por_modelo: Grupo[];
  serie: { minuto: number; requisicoes: number; erros: number }[];
  shadow: Comparacao[];
};
export type Cliente = {
  id: number;
  nome: string;
  rpm: number;
  orcamento_micro: number;
  cache: boolean;
  lgpd: boolean;
  modelos: string[] | null;
};
export type Chave = { id: number; cliente_id: number; prefixo: string; criada_em: string; revogada_em: string | null };
export type Modelo = { alias: string; destinos: string[]; shadow?: string };
// O breaker é por cliente: "breaker" é o pior estado entre eles, "abertos" quantos não estão fechados.
export type EstadoDestino = { destino: string; breaker: string; abertos: number };

export class ErroApi extends Error {
  constructor(
    public status: number,
    mensagem: string,
  ) {
    super(mensagem);
  }
}

// O cookie HttpOnly do admin vai sozinho (mesma origem); nada de token no localStorage.
export async function api<T>(caminho: string, init?: RequestInit): Promise<T> {
  const r = await fetch(caminho, {
    ...init,
    credentials: "same-origin",
    headers: init?.body ? { "Content-Type": "application/json" } : undefined,
  });
  if (!r.ok) {
    const corpo = await r.json().catch(() => null);
    throw new ErroApi(r.status, corpo?.erro?.mensagem ?? `Erro ${r.status}`);
  }
  return (r.status === 204 ? undefined : await r.json()) as T;
}

export const enviar = (metodo: string, caminho: string, corpo?: unknown) =>
  api(caminho, { method: metodo, body: corpo === undefined ? undefined : JSON.stringify(corpo) });

// Custos de LLM por chamada são frações de centavo: abaixo de 1 centavo, 2 dígitos significativos.
const dolar = new Intl.NumberFormat("en-US", { style: "currency", currency: "USD" });
const centavos = new Intl.NumberFormat("en-US", { style: "currency", currency: "USD", maximumSignificantDigits: 2 });
export const usd = (micro: number) => (micro > 0 && micro < 10_000 ? centavos : dolar).format(micro / 1e6);
export const num = (n: number) => n.toLocaleString("pt-BR");
