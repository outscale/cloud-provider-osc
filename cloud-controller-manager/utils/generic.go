package utils

func Map[E, F any](in []E, fn func(item E) (F, bool)) []F {
	out := make([]F, 0, len(in))
	for i := range in {
		add, ok := fn(in[i])
		if !ok {
			continue
		}
		out = append(out, add)
	}
	return out
}

func ToList[E comparable, F, G any](in map[E]F, fn func(key E, value F) G) []G {
	out := make([]G, 0, len(in))
	for k, v := range in {
		out = append(out, fn(k, v))
	}
	return out
}
