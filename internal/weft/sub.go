package weft

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultTemplate is what a new bundle starts from. weft fills the `PROXY`
// group with the bundle's members and the `AUTO` group with every node.
const DefaultTemplate = `mixed-port: 7890
allow-lan: false
mode: rule
log-level: info
dns:
  enable: true
  ipv6: false
  nameserver:
    - 223.5.5.5
    - 119.29.29.29
  fallback:
    - https://1.1.1.1/dns-query
proxies: []
proxy-groups:
  - name: PROXY
    type: select
    proxies: []
  - name: AUTO
    type: url-test
    url: https://www.gstatic.com/generate_204
    interval: 300
    tolerance: 50
    proxies: []
rules:
  - GEOIP,CN,DIRECT
  - MATCH,PROXY
`

// bundleNode is a node with its name already namespaced to its host.
type bundleNode struct {
	HostName string
	NodeName string
	Display  string
	Type     string
	Alive    bool
	DelayMS  int
	Raw      map[string]any
}

func namespaced(raw json.RawMessage, hostName, nodeName string) (bundleNode, bool) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil || m == nil {
		return bundleNode{}, false
	}
	display := hostName + "/" + nodeName
	m["name"] = display
	typ, _ := m["type"].(string)
	return bundleNode{
		HostName: hostName,
		NodeName: nodeName,
		Display:  display,
		Type:     strings.ToUpper(typ),
		Raw:      m,
	}, true
}

// BuildMihomoYAML renders a full mihomo config for the bundle. The template is
// edited as a YAML node tree so its key order and comments-of-shape survive.
func BuildMihomoYAML(b Bundle, hosts []Host, nodes []Node) ([]byte, error) {
	hostNames := map[string]string{}
	for _, h := range hosts {
		hostNames[h.ID] = h.Name
	}
	var bn []bundleNode
	for _, n := range nodes {
		item, ok := namespaced(n.Raw, hostNames[n.HostID], n.Name)
		if !ok {
			continue
		}
		item.Alive = n.Alive
		item.DelayMS = n.DelayMS
		bn = append(bn, item)
	}
	sort.SliceStable(bn, func(i, j int) bool {
		if bn[i].HostName != bn[j].HostName {
			return bn[i].HostName < bn[j].HostName
		}
		return bn[i].Display < bn[j].Display
	})

	tpl := b.Template
	if strings.TrimSpace(tpl) == "" {
		tpl = DefaultTemplate
	}

	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(tpl), &doc); err != nil {
		return nil, fmt.Errorf("template is not valid yaml: %w", err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("template must be a yaml mapping")
	}
	root := doc.Content[0]

	all := make([]string, 0, len(bn))
	proxies := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, n := range bn {
		all = append(all, n.Display)
		proxies.Content = append(proxies.Content, nodeOf(n.Raw))
	}

	entry, groups := groupPlan(b, bn, all)
	setKeyBefore(root, "proxies", "proxy-groups", proxies)

	groupsNode, ok := valueOf(root, "proxy-groups")
	if !ok || groupsNode.Kind != yaml.SequenceNode || len(groupsNode.Content) == 0 {
		setKey(root, "proxy-groups", nodeOf(groups))
	} else {
		present := map[string]bool{}
		for _, g := range groupsNode.Content {
			if g.Kind != yaml.MappingNode {
				continue
			}
			nameNode, _ := valueOf(g, "name")
			present[nameNode.Value] = true
			switch nameNode.Value {
			case "PROXY":
				setKey(g, "proxies", nodeOf(entry))
			case "AUTO":
				setKey(g, "proxies", nodeOf(all))
			}
		}
		// host/type groups the template does not carry yet
		for _, g := range groups {
			gm, ok := g.(map[string]any)
			if !ok {
				continue
			}
			name, _ := gm["name"].(string)
			if present[name] {
				continue
			}
			present[name] = true
			groupsNode.Content = append(groupsNode.Content, nodeOf(g))
		}
	}
	if _, ok := valueOf(root, "rules"); !ok {
		setKey(root, "rules", nodeOf([]string{"MATCH,PROXY"}))
	}
	return yaml.Marshal(&doc)
}

// setKeyBefore writes a mapping value, inserting the pair ahead of `before`
// when the key is new, so a generated config keeps the readable order.
func setKeyBefore(mapping *yaml.Node, key, before string, value *yaml.Node) {
	if _, ok := valueOf(mapping, key); ok {
		setKey(mapping, key, value)
		return
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == before {
			pair := []*yaml.Node{
				{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
				value,
			}
			rest := append([]*yaml.Node{}, mapping.Content[i:]...)
			mapping.Content = append(append(mapping.Content[:i], pair...), rest...)
			return
		}
	}
	setKey(mapping, key, value)
}

// setKey replaces a mapping value in place, or appends the pair.
func setKey(mapping *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = value
			return
		}
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value)
}

func valueOf(mapping *yaml.Node, key string) (*yaml.Node, bool) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1], true
		}
	}
	return nil, false
}

// nodeOf turns any value into yaml nodes so it can be spliced into a template.
func nodeOf(v any) *yaml.Node {
	raw, err := yaml.Marshal(v)
	if err != nil {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
	}
	var n yaml.Node
	if err := yaml.Unmarshal(raw, &n); err != nil || len(n.Content) == 0 {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
	}
	return n.Content[0]
}

func groupPlan(b Bundle, nodes []bundleNode, all []string) ([]any, []any) {
	switch b.GroupBy {
	case "type":
		byType := map[string][]string{}
		for _, n := range nodes {
			byType[n.Type] = append(byType[n.Type], n.Display)
		}
		keys := make([]string, 0, len(byType))
		for k := range byType {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var groups []any
		var entry []any
		for _, k := range keys {
			groups = append(groups, map[string]any{
				"name": k, "type": b.GroupType, "proxies": byType[k],
			})
			entry = append(entry, k)
		}
		entry = append(entry, "AUTO")
		groups = append(groups, map[string]any{"name": "PROXY", "type": "select", "proxies": entry})
		return entry, groups
	case "flat":
		entry := make([]any, 0, len(all)+1)
		for _, n := range all {
			entry = append(entry, n)
		}
		return []any{"AUTO"}, []any{
			map[string]any{"name": "PROXY", "type": "select", "proxies": entry},
		}
	}
	// default: one group per host, ordered by host name
	byHost := map[string][]string{}
	for _, n := range nodes {
		byHost[n.HostName] = append(byHost[n.HostName], n.Display)
	}
	keys := make([]string, 0, len(byHost))
	for k := range byHost {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var groups []any
	var entry []any
	for _, k := range keys {
		groups = append(groups, map[string]any{
			"name": k, "type": b.GroupType, "proxies": byHost[k],
		})
		entry = append(entry, k)
	}
	entry = append(entry, "AUTO")
	groups = append(groups, map[string]any{"name": "PROXY", "type": "select", "proxies": entry})
	return entry, groups
}

// BuildLinks renders share links, one per line. result for target=text; the
// base64 target wraps the same payload.
func BuildLinks(hosts []Host, nodes []Node) []string {
	hostNames := map[string]string{}
	for _, h := range hosts {
		hostNames[h.ID] = h.Name
	}
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		var raw map[string]any
		if err := json.Unmarshal(n.Raw, &raw); err != nil {
			continue
		}
		uri := NodeURI(raw, hostNames[n.HostID]+"/"+n.Name)
		if uri == "" {
			continue
		}
		out = append(out, uri)
	}
	sort.Strings(out)
	return out
}

func base64Std(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func base64NoPad(s string) string {
	return strings.TrimRight(base64.StdEncoding.EncodeToString([]byte(s)), "=")
}
