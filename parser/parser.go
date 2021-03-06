package parser

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"

	"github.com/veino/bitfan/parser/conditionalexpression"
)

type Parser struct {
	l    *lexerStream
	line int
	col  int
}

type Configuration struct {
	Sections map[string]*Section
}

type Section struct {
	Name    string
	Plugins map[int]*Plugin
}

type Plugin struct {
	Name     string
	Label    string
	Codec    *Codec
	Settings map[int]*Setting
	When     map[int]*When // IF and ElseIF with order
}

type Codec struct {
	Name     string
	Settings map[int]*Setting
}

type When struct {
	Expression string          // condition
	Plugins    map[int]*Plugin // actions
}

type Setting struct {
	K string
	V interface{}
}

func NewParser(r io.Reader) *Parser {
	buf := new(bytes.Buffer)
	buf.ReadFrom(r)
	return &Parser{l: newLexerStream(buf.String())}
}

func (p *Parser) Parse() (*Configuration, error) {
	var err error
	var tok Token

	config := &Configuration{
		Sections: map[string]*Section{},
	}

	for {

		tok, err = p.getToken(TokenComment, TokenString, TokenEOF, TokenRCurlyBrace)
		if err != nil {
			return config, fmt.Errorf("parse error Parse %s", err)
		}

		// If Comment Donoe
		if tok.Kind == TokenEOF {
			break
		}

		switch tok.Kind {
		case TokenComment:
			continue
		case TokenString:
			var section *Section
			section, err = p.parseSection(&tok)
			if err != nil {
				return config, err
			}
			config.Sections[section.Name] = section
		}

	}

	return config, nil
}

func (p *Parser) parseSection(tok *Token) (*Section, error) {
	section := &Section{}
	if tok.Value != "input" && tok.Value != "filter" && tok.Value != "output" {
		return section, fmt.Errorf("parse error, unexpected '%s', line %d col %d", tok.Value, tok.Line, tok.Col)
	}

	section.Name = tok.Value.(string)
	section.Plugins = make(map[int]*Plugin, 0)

	var err error
	*tok, err = p.getToken(TokenLCurlyBrace)

	if err != nil {
		return section, fmt.Errorf("section parse error %s", err)
	}
	i := 0
	for {
		*tok, err = p.getToken(TokenComment, TokenString, TokenRCurlyBrace, TokenIf, TokenElse, TokenElseIf)
		if err != nil {
			log.Printf(" -sp- %s %s", GetTokenKindString(tok.Kind), err)
			return section, fmt.Errorf("parse section error %s", err)
		}

		if tok.Kind == TokenRCurlyBrace {
			break
		}

		switch tok.Kind {
		case TokenComment:
			continue
		case TokenString:
			plugin, err := p.parsePlugin(tok)
			if err != nil {
				return section, err
			}
			section.Plugins[i] = plugin
			i = i + 1
			continue
		case TokenIf:
			plugin, err := p.parseWHEN(tok)
			if err != nil {
				return section, err
			}
			section.Plugins[i] = plugin
			i = i + 1
			continue
		case TokenElse:
			plugin, err := p.parseWHEN(tok)
			if err != nil {
				return section, err
			}
			plugin.When[0].Expression = "true"
			iWhen := len(section.Plugins[i-1].When)
			section.Plugins[i-1].When[iWhen] = plugin.When[0]
			continue
		case TokenElseIf:
			plugin, err := p.parseWHEN(tok)
			if err != nil {
				return section, err
			}
			iWhen := len(section.Plugins[i-1].When)
			section.Plugins[i-1].When[iWhen] = plugin.When[0]
			continue
		}
	}

	return section, nil
}

func (p *Parser) parseWHEN(tok *Token) (*Plugin, error) {
	pluginWhen := &Plugin{}
	pluginWhen.Name = "when"
	pluginWhen.When = make(map[int]*When, 0)

	var err error
	var expression string
	expression, err = conditionalexpression.ToWhenExpression(tok.Value.(string))
	if err != nil {
		return pluginWhen, fmt.Errorf("Conditional expression parse error %s", err)
	}

	when := &When{
		Expression: expression,
		Plugins:    map[int]*Plugin{},
	}

	// si pas de { alors erreur

	*tok, err = p.getToken(TokenLCurlyBrace)
	if err != nil {
		return pluginWhen, fmt.Errorf("IF parse error %s", err)
	}
	i := 0
	for {
		*tok, err = p.getToken(TokenComment, TokenString, TokenRCurlyBrace, TokenIf, TokenElse, TokenElseIf)
		if err != nil {
			return pluginWhen, fmt.Errorf("parse IF error %s", err)
		}

		if tok.Kind == TokenRCurlyBrace {
			break
		}

		switch tok.Kind {
		case TokenComment:
			continue
		case TokenString:
			plugin, err := p.parsePlugin(tok)
			if err != nil {
				return pluginWhen, err
			}
			when.Plugins[i] = plugin
			i = i + 1
			continue
		case TokenIf:
			plugin, err := p.parseWHEN(tok)
			if err != nil {
				return pluginWhen, err
			}
			when.Plugins[i] = plugin
			i = i + 1
			continue
		case TokenElse:
			plugin, err := p.parseWHEN(tok)
			if err != nil {
				return pluginWhen, err
			}
			plugin.When[0].Expression = "true"
			iWhen := len(when.Plugins[i-1].When)
			when.Plugins[i-1].When[iWhen] = plugin.When[0]
			continue
		case TokenElseIf:
			plugin, err := p.parseWHEN(tok)
			if err != nil {
				return pluginWhen, err
			}
			iWhen := len(when.Plugins[i-1].When)
			when.Plugins[i-1].When[iWhen] = plugin.When[0]
			continue
		}
	}

	id := len(pluginWhen.When)
	pluginWhen.When[id] = when
	return pluginWhen, nil
}

func (p *Parser) parsePlugin(tok *Token) (*Plugin, error) {
	var err error

	plugin := &Plugin{}
	plugin.Name = tok.Value.(string)
	plugin.Settings = map[int]*Setting{}
	plugin.Codec = &Codec{}

	*tok, err = p.getToken(TokenLCurlyBrace, TokenString)
	if err != nil {
		return plugin, fmt.Errorf("Plugin parse error %s", err)
	}

	if tok.Kind == TokenString {
		plugin.Label = tok.Value.(string)
		*tok, err = p.getToken(TokenLCurlyBrace)
		if err != nil {
			return plugin, fmt.Errorf("Plugin parse error %s", err)
		}
	}

	i := 0
	var advancedTok *Token
	for {
		if advancedTok == nil {
			*tok, err = p.getToken(TokenComment, TokenString, TokenRCurlyBrace, TokenComma)
			if err != nil {
				return plugin, fmt.Errorf("plugin parse error %s", err)
			}
		} else {
			tok = advancedTok
			advancedTok = nil
		}

		if tok.Kind == TokenRCurlyBrace {
			break
		}

		switch tok.Kind {
		case TokenComma:
			continue
		case TokenComment:
			continue
		case TokenString:

			if tok.Value == "codec" {
				codec, rewind, err := p.parseCodec(tok)
				if err != nil {
					return plugin, err
				}
				plugin.Codec = codec
				if rewind != nil {
					advancedTok = rewind
				}

				continue
			}

			setting, err := p.parseSetting(tok)
			if err != nil {
				return plugin, err
			}
			plugin.Settings[i] = setting
			i = i + 1
			continue
		}

	}

	return plugin, nil
}

func (p *Parser) parseCodecSettings(tok *Token) (map[int]*Setting, error) {
	var err error
	settings := make(map[int]*Setting, 0)

	i := 0
	for {
		*tok, err = p.getToken(TokenComment, TokenString, TokenRCurlyBrace)
		if err != nil {
			return settings, fmt.Errorf("codec settings parse error %s", err)
		}

		if tok.Kind == TokenRCurlyBrace {
			break
		}

		switch tok.Kind {
		case TokenComment:
			continue
		case TokenString:
			setting, err := p.parseSetting(tok)
			if err != nil {
				return settings, err
			}
			settings[i] = setting
			i = i + 1
			continue
		}

	}
	return settings, nil
}

func (p *Parser) parseCodec(tok *Token) (*Codec, *Token, error) {
	var err error

	codec := &Codec{}
	codec.Settings = map[int]*Setting{}

	*tok, err = p.getToken(TokenAssignment)
	if err != nil {
		return codec, nil, fmt.Errorf("codec 1 parse error %s", err)
	}

	*tok, err = p.getToken(TokenString)
	if err != nil {
		return codec, nil, fmt.Errorf("codec 2 parse error %s", err)
	}
	codec.Name = tok.Value.(string)

	// rechercher un {
	*tok, err = p.getToken(TokenLCurlyBrace)
	if err != nil {
		return codec, tok, nil
	}

	// il y a un { -> on charge les settings jusqu'au }
	i := 0
	for {
		*tok, err = p.getToken(TokenRCurlyBrace, TokenComment, TokenString, TokenComma)
		if err != nil {
			return codec, nil, fmt.Errorf("plugin parse error %s", err)
		}

		if tok.Kind == TokenRCurlyBrace {
			break
		}

		switch tok.Kind {
		case TokenComma:
			continue
		case TokenComment:
			continue
		case TokenString:
			setting, err := p.parseSetting(tok)
			if err != nil {
				return codec, nil, err
			}
			codec.Settings[i] = setting
			i = i + 1
			continue
		}
	}

	// log.Printf(" -pc- %s %s", TokenType(tok.Kind).String(), tok.Value)
	return codec, nil, nil
}

func (p *Parser) parseSetting(tok *Token) (*Setting, error) {
	setting := &Setting{}

	setting.K = tok.Value.(string)

	var err error
	*tok, err = p.getToken(TokenAssignment)

	if err != nil {
		return setting, fmt.Errorf("Setting 1 parse error %s", err)
	}

	*tok, err = p.getToken(TokenString, TokenNumber, TokenLBracket, TokenLCurlyBrace, TokenBool)
	if err != nil {
		return setting, fmt.Errorf("Setting 2 parse error %s", err)
	}

	switch tok.Kind {
	case TokenBool:
		setting.V = tok.Value.(bool)
	case TokenString:
		setting.V = tok.Value.(string)
	case TokenNumber:
		setting.V = tok.Value
	case TokenLBracket:
		setting.V, err = p.parseArray()
	case TokenLCurlyBrace:
		setting.V, err = p.parseHash()
	}

	return setting, nil

}

func (p *Parser) parseBool(txt string) interface{} {
	var v interface{}
	// var err error
	if txt == "true" {
		v = true
	} else {
		v = false
	}
	return v
}

func (p *Parser) parseNumber(txt string) (interface{}, error) {
	var v interface{}
	var err error
	if strings.Contains(txt, ".") {
		v, err = strconv.ParseFloat(txt, 64)
	} else {
		v, err = strconv.ParseInt(txt, 10, 64)
	}
	return v, err
}

func (p *Parser) parseString(txt string) string {
	var v string
	if strings.HasPrefix(txt, "\"") {
		v = strings.Replace(txt, "\\", "", -1)
		v = strings.TrimPrefix(v, "\"")
		v = strings.TrimSuffix(v, "\"")
	} else {
		v = txt
	}
	return v
}

func (p *Parser) parseHash() (map[string]interface{}, error) {
	hash := map[string]interface{}{}
	for {
		tok, err := p.getToken(TokenComment, TokenRCurlyBrace, TokenString, TokenComma)
		if err != nil {
			log.Fatalf("ParseHash parse error %s", err)
			return nil, err
		}

		if tok.Kind == TokenRCurlyBrace {
			break
		}

		switch tok.Kind {
		case TokenComment:
			continue
		case TokenString:
			set, err := p.parseSetting(&tok)
			if err != nil {
				return hash, err
			}
			hash[set.K] = set.V
		}

	}
	return hash, nil
}

func (p *Parser) parseArray() ([]interface{}, error) {
	var str interface{}

	vals := make([]interface{}, 0, 20)
	for {
		tok, err := p.getToken(TokenComment, TokenString, TokenNumber, TokenComma, TokenRBracket)
		if err != nil {
			return nil, err
		}

		if tok.Kind == TokenRBracket {
			break
		}

		switch tok.Kind {
		case TokenComment:
			continue
		case TokenComma:
			continue
		case TokenNumber:
			str = tok.Value

		case TokenString:
			str = tok.Value.(string)
		}

		vals = append(vals, str)
	}

	return vals, nil
}

func (p *Parser) rewindToken() error {
	return nil
}

func (p *Parser) getToken(types ...TokenKind) (Token, error) {

	tok, err := readToken(p.l)
	if err != nil {
		return Token{}, fmt.Errorf("Illegal token '%s' found line %d col %d ", tok.Value, tok.Line, tok.Col)
	}

	if tok.Kind == TokenIllegal {
		// log.Printf(" -- %s %s", TokenType(tok.Kind).String(), tok.Value)
		return Token{}, fmt.Errorf("Illegal token '%s' found line %d col %d ", tok.Value, tok.Line, tok.Col)
	}

	for _, t := range types {
		if tok.Kind == t {
			return tok, nil
		}
	}

	if len(types) == 1 {
		return tok, fmt.Errorf("unexpected token '%s' expected '%s' on line %d col %d", tok.Value, GetTokenKindString(types[0]), tok.Line, tok.Col)
	}

	list := make([]string, len(types))
	for i, t := range types {
		list[i] = GetTokenKindString(t)
	}

	return tok, fmt.Errorf("unexpected token '%s' expected one of %s on line %d col %d", tok.Value, strings.Join(list, "|"), tok.Line, tok.Col)
}

func DumpTokens(content []byte) {
	var ret []Token
	var token Token
	var stream *lexerStream
	var err error

	stream = newLexerStream(string(content))
	for stream.canRead() {

		token, err = readToken(stream)

		if err != nil {
			fmt.Printf("ERROR %s\n", err)
			return
		}

		if token.Kind == TokenIllegal {
			fmt.Printf("ERROR %s\n", err)
			color := "\033[93m"
			log.Printf("ERROR %4d line %3d:%-2d %s%-20s\033[0m _\033[92m%s\033[0m_", token.Pos, token.Line, token.Col, color, GetTokenKindString(token.Kind), token.Value)
			break
		}

		// state, err = getLexerStateForToken(token.Kind)
		// if err != nil {
		// 	return
		// }
		color := "\033[93m"
		if token.Kind == TokenIf || token.Kind == TokenElseIf || token.Kind == TokenElse {
			color = "\033[1m\033[91m"
		}
		if token.Kind == TokenLBracket || token.Kind == TokenRBracket || token.Kind == TokenRCurlyBrace || token.Kind == TokenLCurlyBrace {
			color = "\033[90m"
		}

		log.Printf("%4d line %3d:%-2d %s%-20s\033[0m _\033[92m%s\033[0m_", token.Pos, token.Line, token.Col, color, GetTokenKindString(token.Kind), token.Value)

		// append this valid token
		ret = append(ret, token)
	}
}
