// Package lgpd replaces Brazilian personal data with placeholders before the text leaves for the
// provider, and puts the real values back in the response (LGPD is Brazil's data protection law).
// It's risk reduction, not a compliance guarantee: names and addresses have no fixed format and
// aren't detected.
package lgpd

import (
	"fmt"
	"regexp"
	"strings"
)

type tipo struct {
	nome   string
	re     *regexp.Regexp
	valido func(digitos string) bool // nil = the format is enough
}

// Order matters: CNPJ and CPF (Brazilian company and individual taxpayer IDs) before card and phone,
// because a CPF without punctuation also looks like a phone number; the check-digit-validated ones win.
var tipos = []tipo{
	{"CNPJ", regexp.MustCompile(`\d{2}\.?\d{3}\.?\d{3}/?\d{4}-?\d{2}`), cnpjValido},
	{"CPF", regexp.MustCompile(`\d{3}\.?\d{3}\.?\d{3}-?\d{2}`), cpfValido},
	{"CARTAO", regexp.MustCompile(`\d(?:[ -]?\d){12,18}`), luhnValido},
	{"EMAIL", regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}`), nil},
	{"TELEFONE", regexp.MustCompile(`(?:\+55\s?)?(?:\([1-9]{2}\)|[1-9]{2})\s?(?:9\d{4}|[2-5]\d{3})[-\s]?\d{4}`), nil},
}

// Mapa (map) lives only during a request: it never goes to logs, cache or the database.
type Mapa struct {
	porValor map[string]string // type+value → placeholder
	valores  map[string]string // placeholder → value
	cont     map[string]int
}

func NovoMapa() *Mapa {
	return &Mapa{porValor: map[string]string{}, valores: map[string]string{}, cont: map[string]int{}}
}

// Len says how many distinct values were masked (for the dashboard metric).
func (m *Mapa) Len() int { return len(m.valores) }

// Mascarar (mask) is deterministic: the same text always produces the same result, which preserves
// the gateway cache and DeepSeek's prefix cache.
func (m *Mapa) Mascarar(s string) string { return m.mascarar(s, s) }

// MascararVarios masks the messages of a request. Placeholders avoid the text of all of them:
// a literal "[CPF_1]" in one message can't become the placeholder for another message's CPF.
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
		// A piece of a bigger number (protocol number, barcode) isn't personal data.
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

// marcador reuses the placeholder for the same value and never uses text the user already wrote:
// if the input contains a literal "[CPF_1]", the real CPF becomes "[CPF_2]". That way restoring
// never replaces something the user typed.
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

// Restaurar (restore) only replaces placeholders created in this request; any other "[X_n]" stays.
func (m *Mapa) Restaurar(s string) string {
	if len(m.valores) == 0 {
		return s
	}
	return m.replacer().Replace(s)
}

// Stream restores a response that arrives in chunks. A placeholder can arrive split
// ("[CP" + "F_1]"), so it holds back the part from the last "[" while it can still be
// the start of a placeholder.
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

// dvCNPJ uses the weights 2..9 repeated from right to left.
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
