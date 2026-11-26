package build

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// legacyFile maps WordPress query URLs (/?p=123, /?page_id=45) to the new
// paths. Pages read it from the home page script when the host cannot run
// the nginx rules.
const legacyFile = "_legacy.json"

// legacyQueries lists the WordPress query parameters that name a post ID.
// WordPress accepts ?p= for pages too.
var legacyQueries = map[string][]string{
	"post": {"p"},
	"page": {"page_id", "p"},
}

// collectLegacy finds published contents imported from WordPress. Their
// number is the WordPress post ID, so /?p=<number> can be redirected.
func (g *generator) collectLegacy() {
	g.legacy = map[string]map[string]string{}
	for _, it := range g.items {
		if it.row.SourceFormat == nil || !strings.HasPrefix(*it.row.SourceFormat, "wordpress") {
			continue
		}
		for _, param := range legacyQueries[it.row.Type] {
			if g.legacy[param] == nil {
				g.legacy[param] = map[string]string{}
			}
			g.legacy[param][strconv.FormatInt(it.row.Number, 10)] = it.path
		}
	}
}

// legacyScript forwards /?p=123 visits on hosts without the nginx rules.
// The core adds it to the home page, whatever the theme.
const legacyScript = `<script>
(function () {
  var q = new URLSearchParams(location.search);
  var key = ["p", "page_id"].find(function (k) { return /^\d+$/.test(q.get(k) || ""); });
  if (!key) return;
  fetch("/_legacy.json").then(function (r) { return r.ok ? r.json() : null; }).then(function (m) {
    var to = m && m[key] && m[key][q.get(key)];
    if (to) location.replace(to);
  });
})();
</script>`

// writeLegacy writes legacyFile and returns nginx rules for the same map.
func (g *generator) writeLegacy() (string, error) {
	if len(g.legacy) == 0 {
		return "", nil
	}
	data, err := json.Marshal(g.legacy)
	if err != nil {
		return "", err
	}
	if err := g.out.write(legacyFile, systemOwner, data); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("# WordPress query URLs (/?p=123). Imported posts keep their WordPress ID as number.\n")
	params := make([]string, 0, len(g.legacy))
	for p := range g.legacy {
		params = append(params, p)
	}
	slices.Sort(params)
	for _, param := range params {
		ids := make([]string, 0, len(g.legacy[param]))
		for id := range g.legacy[param] {
			ids = append(ids, id)
		}
		slices.SortFunc(ids, func(a, b string) int {
			x, _ := strconv.Atoi(a)
			y, _ := strconv.Atoi(b)
			return x - y
		})
		for _, id := range ids {
			// param and id are fixed identifiers and digits; only the target needs quoting.
			fmt.Fprintf(&b, "if ($arg_%s = %s) { return 301 %s; }\n", param, id,
				nginxQuote(strings.ReplaceAll(escapePath(g.legacy[param][id]), "$", "%24")))
		}
	}
	return b.String(), nil
}
