package lgpd

import (
	"os"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

// The corpus marks each piece of personal data inline: "meu CPF é {{CPF:529.982.247-25}}".
// Types the detector doesn't cover (NOME, ENDERECO) are labeled too, so recall over all
// personal data is honest.
var rotulo = regexp.MustCompile(`\{\{([A-Z]+):([^}]+)\}\}`)

type dado struct{ tipo, valor string }

type exemplo struct {
	linha int
	texto string
	dados []dado
}

// extrair dedups like Mapa does: the same value twice in a line is one piece of data.
func extrair(linha string) (string, []dado) {
	linha = strings.TrimSpace(linha)
	var dados []dado
	visto := map[dado]bool{}
	for _, m := range rotulo.FindAllStringSubmatch(linha, -1) {
		d := dado{m[1], m[2]}
		if !visto[d] {
			visto[d] = true
			dados = append(dados, d)
		}
	}
	return rotulo.ReplaceAllString(linha, "$2"), dados
}

func lerCorpus(t *testing.T, caminho string) []exemplo {
	t.Helper()
	b, err := os.ReadFile(caminho)
	if err != nil {
		t.Fatal(err)
	}
	var out []exemplo
	for i, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		texto, dados := extrair(l)
		out = append(out, exemplo{linha: i + 1, texto: texto, dados: dados})
	}
	return out
}

// detectados runs the real masking and reads back what it replaced. The type is the
// placeholder's prefix: "[CPF_2]" → "CPF".
func detectados(texto string) []dado {
	m := NovoMapa()
	m.Mascarar(texto)
	var out []dado
	for mk, v := range m.valores {
		out = append(out, dado{mk[1:strings.LastIndexByte(mk, '_')], v})
	}
	return out
}

func TestExtrair(t *testing.T) {
	casos := []struct {
		nome, linha, texto string
		dados              []dado
	}{
		{"no tags", "pedido 123", "pedido 123", nil},
		{"one tag", "CPF {{CPF:529.982.247-25}} ok", "CPF 529.982.247-25 ok", []dado{{"CPF", "529.982.247-25"}}},
		{"two types", "{{EMAIL:a@exemplo.com}} e {{NOME:Ana Lima}}", "a@exemplo.com e Ana Lima",
			[]dado{{"EMAIL", "a@exemplo.com"}, {"NOME", "Ana Lima"}}},
		{"duplicate value counted once", "{{CPF:529.982.247-25}} {{CPF:529.982.247-25}}",
			"529.982.247-25 529.982.247-25", []dado{{"CPF", "529.982.247-25"}}},
		{"crlf", "fone {{TELEFONE:(11) 98765-4321}}\r", "fone (11) 98765-4321", []dado{{"TELEFONE", "(11) 98765-4321"}}},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			texto, dados := extrair(c.linha)
			if texto != c.texto || !reflect.DeepEqual(dados, c.dados) {
				t.Fatalf("extrair(%q) = %q %v, want %q %v", c.linha, texto, dados, c.texto, c.dados)
			}
		})
	}
}

func TestDetectados(t *testing.T) {
	got := detectados("CPF 529.982.247-25, a@exemplo.com")
	want := map[dado]bool{{"CPF", "529.982.247-25"}: true, {"EMAIL", "a@exemplo.com"}: true}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for _, d := range got {
		if !want[d] {
			t.Fatalf("unexpected %v in %v", d, got)
		}
	}
}

func TestPontuar(t *testing.T) {
	cpf, tel := dado{"CPF", "529.982.247-25"}, dado{"TELEFONE", "52998224725"}
	casos := []struct {
		nome               string
		esperados, achados []dado
		quer               map[string]placar
	}{
		{"hit", []dado{cpf}, []dado{cpf}, map[string]placar{"CPF": {vp: 1}}},
		{"miss", []dado{cpf}, nil, map[string]placar{"CPF": {fn: 1}}},
		{"false alarm", nil, []dado{cpf}, map[string]placar{"CPF": {fp: 1}}},
		{"wrong type", []dado{{"CPF", "52998224725"}}, []dado{tel},
			map[string]placar{"CPF": {fn: 1}, "TELEFONE": {fp: 1}}},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			total := map[string]*placar{}
			pontuar(total, c.esperados, c.achados)
			if len(total) != len(c.quer) {
				t.Fatalf("types = %v, want %v", total, c.quer)
			}
			for tipo, q := range c.quer {
				if total[tipo] == nil || *total[tipo] != q {
					t.Fatalf("%s = %+v, want %+v", tipo, total[tipo], q)
				}
			}
		})
	}
}

// placar counts hits and misses for one data type.
type placar struct{ vp, fp, fn int } // true positives, false positives, false negatives

// pontuar compares what was expected with what was found, by type and value. A CPF found as
// TELEFONE is a miss for CPF and a false alarm for TELEFONE, never a hit.
func pontuar(total map[string]*placar, esperados, achados []dado) (falsos, perdidos []dado) {
	conta := func(tipo string) *placar {
		if total[tipo] == nil {
			total[tipo] = &placar{}
		}
		return total[tipo]
	}
	achou := map[dado]bool{}
	for _, d := range achados {
		achou[d] = true
	}
	esperado := map[dado]bool{}
	for _, d := range esperados {
		esperado[d] = true
		if achou[d] {
			conta(d.tipo).vp++
		} else {
			conta(d.tipo).fn++
			perdidos = append(perdidos, d)
		}
	}
	for _, d := range achados {
		if !esperado[d] {
			conta(d.tipo).fp++
			falsos = append(falsos, d)
		}
	}
	return falsos, perdidos
}

// precisao: of what it masked, how much was personal data. 1 when it masked nothing.
func (p placar) precisao() float64 {
	if p.vp+p.fp == 0 {
		return 1
	}
	return float64(p.vp) / float64(p.vp+p.fp)
}

// recall: of the personal data in the text, how much it masked. 1 when there was nothing to find.
func (p placar) recall() float64 {
	if p.vp+p.fn == 0 {
		return 1
	}
	return float64(p.vp) / float64(p.vp+p.fn)
}

// cobertos are the types the detector claims to handle; only they are gated.
var cobertos = []string{"CNPJ", "CPF", "CARTAO", "EMAIL", "TELEFONE"}

// Floors pinned at the measured values (a ratchet): a regression fails the build; an improvement
// raises the floor in the same commit.
const (
	minPrecisao = 0.96
	minRecall   = 0.90
)

func TestQualidadeDeteccao(t *testing.T) {
	exemplos := lerCorpus(t, "testdata/corpus.txt")
	if len(exemplos) < 40 {
		t.Fatalf("corpus has %d examples, want at least 40", len(exemplos))
	}
	total := map[string]*placar{}
	for _, ex := range exemplos {
		falsos, perdidos := pontuar(total, ex.dados, detectados(ex.texto))
		for _, d := range falsos {
			t.Logf("line %d: false positive %s %q", ex.linha, d.tipo, d.valor)
		}
		for _, d := range perdidos {
			t.Logf("line %d: missed %s %q", ex.linha, d.tipo, d.valor)
		}
	}

	tipos := make([]string, 0, len(total))
	for k := range total {
		tipos = append(tipos, k)
	}
	sort.Strings(tipos)
	var cob, todos placar
	t.Logf("%-9s %4s %4s %4s %9s %7s", "type", "TP", "FP", "FN", "precision", "recall")
	for _, k := range tipos {
		p := *total[k]
		t.Logf("%-9s %4d %4d %4d %9.2f %7.2f", k, p.vp, p.fp, p.fn, p.precisao(), p.recall())
		todos.vp, todos.fp, todos.fn = todos.vp+p.vp, todos.fp+p.fp, todos.fn+p.fn
		if slices.Contains(cobertos, k) {
			cob.vp, cob.fp, cob.fn = cob.vp+p.vp, cob.fp+p.fp, cob.fn+p.fn
		}
	}
	t.Logf("covered types:     precision %.2f, recall %.2f", cob.precisao(), cob.recall())
	t.Logf("all personal data: recall %.2f (names and addresses aren't detected)", todos.recall())

	if cob.precisao() < minPrecisao || cob.recall() < minRecall {
		t.Errorf("covered types fell below the floor: precision %.3f (min %.2f), recall %.3f (min %.2f)",
			cob.precisao(), minPrecisao, cob.recall(), minRecall)
	}
}
