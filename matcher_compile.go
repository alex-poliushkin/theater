package theater

import "fmt"

type matcherCompileResolver struct {
	resolver MatcherResolver
}

func newMatcherCompileResolver(resolver MatcherResolver) MatcherCompileContext {
	return matcherCompileResolver{resolver: resolver}
}

func (c matcherCompileResolver) Compile(ref string, args Values) (Matcher, error) {
	descriptor, err := c.resolver.Resolve(ref)
	if err != nil {
		return nil, err
	}

	return descriptor.Compile(c, args)
}

func (c matcherCompileResolver) ResolveSugarKey(key string) (MatcherDescriptor, error) {
	resolver, ok := c.resolver.(MatcherSugarResolver)
	if !ok {
		return MatcherDescriptor{}, fmt.Errorf("matcher sugar %q is not supported", key)
	}

	return resolver.ResolveSugarKey(key)
}
