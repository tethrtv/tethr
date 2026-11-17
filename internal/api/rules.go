package api

import (
	"context"
	"errors"
	"strings"

	"github.com/tethrtv/tethr/internal/core"
)

func ruleProblem(err error) (ProblemJSONResponse, bool) {
	switch {
	case errors.Is(err, core.ErrRule):
		_, why, _ := strings.Cut(err.Error(), ": ")
		return problem("invalid_rule", "That rule can’t be used: "+why+"."), true
	case errors.Is(err, core.ErrRuleList):
		return problem("rule_list", "A rule list’s titles come from its rule. They can be taken out, but not added, reordered, given notes or picked as the cover, and a list made by hand can’t be given a rule."), true
	case errors.Is(err, core.ErrRuleSort):
		return problem("invalid_sort", "Rule lists sort by rank, title or year."), true
	}
	return ProblemJSONResponse{}, false
}

func strs[T ~string](p *[]T) []string {
	if p == nil {
		return nil
	}
	out := make([]string, 0, len(*p))
	for _, v := range *p {
		out = append(out, string(v))
	}
	return out
}

func fromValues(v *RuleValues) core.Values {
	if v == nil {
		return core.Values{}
	}
	return core.Values{Any: strs(v.Any), None: strs(v.None)}
}

func fromRange(r *RuleRange) core.Between {
	if r == nil {
		return core.Between{}
	}
	return core.Between{From: r.From, To: r.To}
}

func fromRule(r *ListRule) *core.Rule {
	if r == nil {
		return nil
	}
	out := core.Rule{
		Genres: fromValues(r.Genres), Types: fromValues(r.Types), Studios: fromValues(r.Studios), Networks: fromValues(r.Networks),
		Countries: fromValues(r.Countries), Languages: fromValues(r.Languages), Year: fromRange(r.Year), Runtime: fromRange(r.Runtime),
		Watched: r.Watched, Rated: r.Rated, Watchlist: r.Watchlist,
	}
	if r.Kind != nil {
		out.Kind = string(*r.Kind)
	}
	if r.LeftOut != nil {
		out.LeftOut = *r.LeftOut
	}
	return &out
}

func toValues(v core.Values) *RuleValues {
	if len(v.Any) == 0 && len(v.None) == 0 {
		return nil
	}
	out := RuleValues{}
	if len(v.Any) > 0 {
		out.Any = &v.Any
	}
	if len(v.None) > 0 {
		out.None = &v.None
	}
	return &out
}

func toRange(r core.Between) *RuleRange {
	if r.From == nil && r.To == nil {
		return nil
	}
	return &RuleRange{From: r.From, To: r.To}
}

func toRule(r *core.Rule, names map[string]string) *ListRule {
	if r == nil {
		return nil
	}
	out := ListRule{
		Names:  &names,
		Genres: toValues(r.Genres), Types: toValues(r.Types), Studios: toValues(r.Studios), Networks: toValues(r.Networks),
		Countries: toValues(r.Countries), Languages: toValues(r.Languages), Year: toRange(r.Year), Runtime: toRange(r.Runtime),
		Watched: r.Watched, Rated: r.Rated, Watchlist: r.Watchlist,
	}
	if r.Kind != "" {
		kind := ListRuleKind(r.Kind)
		out.Kind = &kind
	}
	if len(r.LeftOut) > 0 {
		out.LeftOut = &r.LeftOut
	}
	return &out
}

func (s *Server) PreviewRule(ctx context.Context, req PreviewRuleRequestObject) (PreviewRuleResponseObject, error) {
	viewer, ok, err := s.reader(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return PreviewRule404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return PreviewRule401JSONResponse(notConnected), err
	}
	p := req.Params
	rule := core.Rule{
		Genres:    core.Values{Any: strs(p.Genre), None: strs(p.NotGenre)},
		Types:     core.Values{Any: strs(p.Type), None: strs(p.NotType)},
		Studios:   core.Values{Any: strs(p.Studio), None: strs(p.NotStudio)},
		Networks:  core.Values{Any: strs(p.Network), None: strs(p.NotNetwork)},
		Countries: core.Values{Any: strs(p.Country), None: strs(p.NotCountry)},
		Languages: core.Values{Any: strs(p.Language), None: strs(p.NotLanguage)},
		Year:      core.Between{From: p.YearFrom, To: p.YearTo},
		Runtime:   core.Between{From: p.RuntimeFrom, To: p.RuntimeTo},
		Watched:   p.Watched,
		Rated:     p.Rated,
		Watchlist: p.Watchlist,
	}
	if p.Kind != nil {
		rule.Kind = string(*p.Kind)
	}
	query := core.ListQuery{Page: core.Page{Page: max(value(p.Page), 1), Limit: min(max(value(p.Limit), 0), 1000)}}
	if query.Page.Limit == 0 {
		query.Page.Limit = 100
	}
	if p.SortBy != nil {
		query.SortBy = string(*p.SortBy)
	}
	if p.SortHow != nil {
		query.SortHow = string(*p.SortHow)
	}
	preview, err := core.PreviewRule(ctx, s.DB, viewer, rule, value(p.List), query)
	if p, ok := ruleProblem(err); ok {
		return PreviewRule400JSONResponse{p}, nil
	}
	if errors.Is(err, core.ErrNotFound) {
		return PreviewRule404JSONResponse(noSuchList), nil
	}
	if err != nil {
		return nil, err
	}
	out := PreviewRule200JSONResponse{
		Items:        make([]ListItem, 0, len(preview.Entries)),
		MatchedCount: int64(preview.Matched),
		Names:        &preview.Names,
		Total:        min(preview.Matched, 1000),
		Page:         query.Page.Page,
		Limit:        query.Page.Limit,
	}
	for _, e := range preview.Entries {
		item := ListItem{Id: e.ID, Rank: int(e.Rank), ListedAt: e.At, Item: toItem(e.Item)}
		if e.Progress != nil {
			item.Progress = &ItemProgress{Watched: e.Progress.Watched, Total: e.Progress.Total, LastWatchedAt: e.Progress.LastWatchedAt}
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

func (s *Server) CopyList(ctx context.Context, req CopyListRequestObject) (CopyListResponseObject, error) {
	c, ok, err := s.profileCaller(ctx, req.Params.ProfileId)
	if errors.Is(err, core.ErrNotFound) {
		return CopyList404JSONResponse(noSuchProfile), nil
	}
	if !ok {
		return CopyList401JSONResponse(notConnected), err
	}
	var name string
	if req.Body != nil {
		name = value(req.Body.Name)
	}
	l, err := core.CopyRuleList(ctx, s.DB, c.ProfileID, req.Id, name)
	if errors.Is(err, core.ErrNotFound) {
		return CopyList404JSONResponse(problem("not_found", "There’s no rule list with that ID, or it isn’t shared with you.")), nil
	}
	if p, status, ok := listProblem(err); ok {
		if status == 422 {
			return CopyList422JSONResponse(p), nil
		}
		return CopyList400JSONResponse{p}, nil
	}
	if err != nil {
		return nil, err
	}
	return CopyList201JSONResponse(s.toList(l)), nil
}

func (s *Server) SearchCompanies(ctx context.Context, req SearchCompaniesRequestObject) (SearchCompaniesResponseObject, error) {
	found, err := core.Companies(ctx, s.DB, req.Params.Q)
	if err != nil {
		return nil, err
	}
	out := make(SearchCompanies200JSONResponse, 0, len(found))
	for _, c := range found {
		out = append(out, CompanyMatch{Wikidata: c.QID, Name: c.Name, StudioOf: c.StudioOf, NetworkOf: c.NetworkOf})
	}
	return out, nil
}
