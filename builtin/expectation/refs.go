package expectation

// Stable refs for built-in matcher descriptors.
const (
	EqualRef    = "expectation.equal"
	ContainsRef = "expectation.contains"
	MatchesRef  = "expectation.matches"
	NotRef      = "expectation.not"
	PresentRef  = "expectation.present"
	NullRef     = "expectation.null"
	NotNullRef  = "expectation.not_null"
	GTRef       = "expectation.gt"
	GTERef      = "expectation.gte"
	LTRef       = "expectation.lt"
	LTERef      = "expectation.lte"
	BetweenRef  = "expectation.between"
	HasItemRef  = "expectation.has_item"
	AllItemsRef = "expectation.all_items"
	HasKeyRef   = "expectation.has_key"
	LacksKeyRef = "expectation.lacks_key"
	HasEntryRef = "expectation.has_entry"
)
