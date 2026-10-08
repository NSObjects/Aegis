package portfolio

import (
	"errors"
	"fmt"
	"time"
)

// ValidateAt enforces the observed portfolio's time contract against the
// research cutoff. maxAge == 0 disables the optional freshness policy;
// callers must explicitly choose an acceptable age for their use case.
//
// This does not prove that the portfolio was obtained from a broker. It only
// validates timestamps and states actually supplied by the caller.
func ValidateAt(book *Context, requestedAsOf time.Time, maxAge time.Duration) error {
	if err := Validate(book); err != nil {
		return err
	}
	if requestedAsOf.IsZero() {
		return errors.New("research requested_as_of is required for portfolio review")
	}
	if maxAge < 0 {
		return errors.New("portfolio max age must not be negative")
	}
	if book.State == StateNotProvided {
		// Unknown is never silently converted into a fresh empty portfolio.
		return nil
	}

	observedAsOf, err := time.Parse(time.RFC3339, book.AsOf)
	if err != nil {
		return fmt.Errorf("parse portfolio as_of: %w", err)
	}
	if observedAsOf.After(requestedAsOf) {
		return fmt.Errorf(
			"portfolio as_of %q is after research requested_as_of %q",
			book.AsOf, requestedAsOf.Format(time.RFC3339),
		)
	}
	if maxAge > 0 && requestedAsOf.Sub(observedAsOf) > maxAge {
		return fmt.Errorf(
			"portfolio snapshot is stale: as_of %q is older than max age %s at research cutoff %q",
			book.AsOf, maxAge, requestedAsOf.Format(time.RFC3339),
		)
	}
	return nil
}
