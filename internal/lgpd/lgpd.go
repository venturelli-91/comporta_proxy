// Package lgpd troca dados pessoais brasileiros por marcadores antes de o texto sair para o
// provedor, e devolve os valores reais na resposta. É redução de risco, não garantia de
// conformidade: nomes e endereços não têm formato fixo e não são detectados.
package lgpd

import (
	"fmt"
	"regexp"
	"strings"
)

type tipo struct {
	nome   string
	re     *regexp.Regexp
	valido func(digitos string) bool // nil = o formato basta
}

// A ordem importa: CNPJ e CPF antes do cartão e do telefone, porque um CPF sem pontuação
// também tem cara de telefone; os validados por dígito verificador ganham a disputa.
var tipos = []tipo{
	{"CNPJ", regexp.MustCompile(`\d{2}\.?\d{3}\.?\d{3}/?\d{4}-?\d{2}`), cnpjValido},
	{"CPF", regexp.MustCompile(`\d{3}\.?\d{3}\.?\d{3}-?\d{2}`), cpfValido},
	{"CARTAO", regexp.MustCompile(`\d(?:[ -]?\d){12,18}`), luhnValido},
	{"EMAIL", regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}`), nil},
	{"TELEFONE", regexp.MustCompile(`(?:\+55\s?)?(?:\([1-9]{2}\)|[1-9]{2})\s?(?:9\d{4}|[2-5]\d{3})[-\s]?\d{4}`), nil},
}

// Mapa vive só durante uma requisição: nunca vai para log, cache ou banco.
type Mapa struct {
	porValor map[string]string // tipo+valor → marcador
	valores  map[string]string // marcador → valor
	cont     map[string]int
}

func NovoMapa() *Mapa {
	return &Mapa{porValor: map[string]string{}, valores: map[string]string{}, cont: map[string]int{}}
}

// Len diz quantos valores distintos foram mascarados (para a métrica do painel).
func (m *Mapa) Len() int { return len(m.valores) }

// Mascarar é determinístico: o mesmo texto gera sempre o mesmo resultado, o que preserva o
// cache do gateway e o cache de prefixo da DeepSeek.
func (m *Mapa) Mascarar(s string) string { return m.mascarar(s, s) }

// MascararVarios mascara as mensagens de uma requisição. Os marcadores evitam o texto de
// todas elas: um "[CPF_1]" literal numa mensagem não pode virar o marcador do CPF de outra.
func (m *Mapa) MascararVarios(textos []string) []string {
	todos := strings.Join(textos, "\x00")
	out := make([]string, len(textos))
	for i, t := range textos {
		out[i] = m.mascarar(t, todos)
	}
	return out
}

func (m *Mapa) mascarar(s, reservado string) string {
	out := s
	for _, t := range tipos {
		out = m.substituir(out, reservado, t)
	}
	return out
}

func (m *Mapa) substituir(texto, original string, t tipo) string {
	idx := t.re.FindAllStringIndex(texto, -1)
	if idx == nil {
		return texto
	}
	var b strings.Builder
	ult := 0
	for _, p := range idx {
		i, j := p[0], p[1]
		v := texto[i:j]
		// Pedaço de um número maior (protocolo, código de barras) não é dado pessoal.
		if (i > 0 && ehDigito(texto[i-1])) || (j < len(texto) && ehDigito(texto[j])) {
			continue
		}
		if t.valido != nil && !t.valido(soDigitos(v)) {
			continue
		}
		b.WriteString(texto[ult:i])
		b.WriteString(m.marcador(t.nome, v, original))
		ult = j
	}
	b.WriteString(texto[ult:])
	return b.String()
}

// marcador reaproveita o marcador do mesmo valor e nunca usa um texto que o usuário já
// escreveu: se a entrada contém "[CPF_1]" literal, o CPF real vira "[CPF_2]". Assim restaurar
// nunca troca algo que o usuário digitou.
func (m *Mapa) marcador(nome, valor, original string) string {
	chave := nome + "\x00" + valor
	if mk, ok := m.porValor[chave]; ok {
		return mk
	}
	var mk string
	for {
		m.cont[nome]++
		mk = fmt.Sprintf("[%s_%d]", nome, m.cont[nome])
		if !strings.Contains(original, mk) {
			break
		}
	}
	m.porValor[chave] = mk
	m.valores[mk] = valor
	return mk
}

func (m *Mapa) replacer() *strings.Replacer {
	pares := make([]string, 0, 2*len(m.valores))
	for mk, v := range m.valores {
		pares = append(pares, mk, v)
	}
	return strings.NewReplacer(pares...)
}

// Restaurar troca só os marcadores criados nesta requisição; qualquer outro "[X_n]" fica.
func (m *Mapa) Restaurar(s string) string {
	if len(m.valores) == 0 {
		return s
	}
	return m.replacer().Replace(s)
}

// Stream restaura uma resposta que chega em pedaços. Um marcador pode vir partido
// ("[CP" + "F_1]"), então segura o trecho a partir do último "[" enquanto ele ainda
// pode ser o começo de um marcador.
type Stream struct {
	r   *strings.Replacer
	max int
	buf string
}

func (m *Mapa) NovoStream() *Stream {
	s := &Stream{r: m.replacer()}
	for mk := range m.valores {
		s.max = max(s.max, len(mk))
	}
	return s
}

func (s *Stream) Escrever(pedaco string) string {
	s.buf += pedaco
	corte := len(s.buf)
	if i := strings.LastIndexByte(s.buf, '['); i >= 0 && len(s.buf)-i < s.max && !strings.Contains(s.buf[i:], "]") {
		corte = i
	}
	pronto := s.buf[:corte]
	s.buf = s.buf[corte:]
	return s.r.Replace(pronto)
}

func (s *Stream) Finalizar() string {
	resto := s.buf
	s.buf = ""
	return s.r.Replace(resto)
}

func ehDigito(c byte) bool { return c >= '0' && c <= '9' }

func soDigitos(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}

func repetido(d string) bool { return strings.Count(d, d[:1]) == len(d) }

func cpfValido(d string) bool {
	if len(d) != 11 || repetido(d) {
		return false
	}
	return dvCPF(d[:9], 10) == d[9] && dvCPF(d[:10], 11) == d[10]
}

func dvCPF(d string, peso int) byte {
	soma := 0
	for i := range d {
		soma += int(d[i]-'0') * (peso - i)
	}
	r := soma * 10 % 11
	if r == 10 {
		r = 0
	}
	return byte('0' + r)
}

func cnpjValido(d string) bool {
	if len(d) != 14 || repetido(d) {
		return false
	}
	return dvCNPJ(d[:12]) == d[12] && dvCNPJ(d[:13]) == d[13]
}

// dvCNPJ usa os pesos 2..9 repetidos da direita para a esquerda.
func dvCNPJ(d string) byte {
	soma, peso := 0, 2
	for i := len(d) - 1; i >= 0; i-- {
		soma += int(d[i]-'0') * peso
		if peso++; peso > 9 {
			peso = 2
		}
	}
	r := soma % 11
	if r < 2 {
		return '0'
	}
	return byte('0' + 11 - r)
}

func luhnValido(d string) bool {
	if len(d) < 13 || len(d) > 19 {
		return false
	}
	soma := 0
	for i := range d {
		n := int(d[len(d)-1-i] - '0')
		if i%2 == 1 {
			if n *= 2; n > 9 {
				n -= 9
			}
		}
		soma += n
	}
	return soma%10 == 0
}
