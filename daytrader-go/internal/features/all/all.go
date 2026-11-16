// Package all links every feature area into the binary so their init() registrations run.
package all

import (
	_ "github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/features/account"
	_ "github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/features/auth"
	_ "github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/features/market"
	_ "github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/features/rest"
	_ "github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/features/trading"
)
