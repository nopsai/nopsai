package nopsai

import (
	"context"
	"fmt"
	"net/http"

	"nopsai/pkg/buildinfo"
	"nopsai/pkg/license"
	"nopsai/services/nopsai/pkg/audit"
)

// enforceEntitlement decides whether one more of a limited resource may be
// created.
//
// Most installations have nothing to enforce. NopsAI is free and uncapped for
// non-commercial use, so an installation with no commercial key carries no
// ceiling on any resource and this returns immediately. Limits exist only where
// a commercial licence recorded one.
//
// Enforcement also only applies to builds that carry a licence verification
// key. A build compiled without one cannot tell a commercially licensed
// installation from any other, so applying ceilings there would penalise the
// build configuration rather than the operator.
//
// Where a limit does exist it fails closed: if the current count cannot be
// read, the action is refused rather than allowed, because being unable to
// evaluate a limit is not the same as being under it.
func (a *App) enforceEntitlement(ctx context.Context, resource string, limit func(license.Entitlement) int, allows func(license.Entitlement, int) bool, count func(setupCounts) int) error {
	if _, buildCanVerify := license.ParsePublicKey(buildinfo.LicensePublicKey); !buildCanVerify {
		return nil
	}

	entitlement := a.currentEntitlement()

	// A limit of zero means unlimited, which is what the non-commercial licence
	// and an unscoped commercial licence both resolve to. There is no ceiling
	// to evaluate, so there is nothing to fail closed on.
	if limit(entitlement) <= 0 {
		return nil
	}

	counts, err := a.setupCounts(ctx)
	if err != nil {
		return entitlementError{
			resource: resource,
			message:  fmt.Sprintf("Current %s usage could not be read, so the licence limit could not be checked.", resource),
		}
	}
	if allows(entitlement, count(counts)) {
		return nil
	}

	return entitlementError{
		resource: resource,
		message: fmt.Sprintf(
			"This installation has reached the %s limit recorded in its commercial licence. Contact contact@nopsai.com to raise it.",
			resource,
		),
	}
}

type entitlementError struct {
	resource string
	message  string
}

func (e entitlementError) Error() string { return e.message }

// writeEntitlementError answers 402 Payment Required, which says precisely what
// happened: the request is well formed and authorized, and the installation is
// not entitled to it.
func (a *App) writeEntitlementError(w http.ResponseWriter, r *http.Request, err error) bool {
	limitErr, ok := err.(entitlementError)
	if !ok {
		return false
	}
	a.auditEntitlementDenial(r, limitErr.resource, limitErr.message)
	http.Error(w, limitErr.message, http.StatusPaymentRequired)
	return true
}

func (a *App) auditEntitlementDenial(r *http.Request, resource, reason string) {
	if a == nil || a.auditLogger == nil || r == nil {
		return
	}
	_ = a.auditLogger.Write(r.Context(), audit.Entry{
		Action:   "system.license.denied",
		Resource: "system:license",
		Result:   "denied",
		ActorSub: actorIDFromRequest(r),
		Metadata: map[string]any{"resource": resource, "reason": reason},
	})
}

func (a *App) enforceUserEntitlement(ctx context.Context) error {
	return a.enforceEntitlement(ctx, "user",
		func(e license.Entitlement) int { return e.Claims.MaxUsers },
		func(e license.Entitlement, current int) bool { return e.AllowsUsers(current) },
		func(c setupCounts) int { return c.Users },
	)
}

func (a *App) enforceTeamEntitlement(ctx context.Context) error {
	return a.enforceEntitlement(ctx, "team",
		func(e license.Entitlement) int { return e.Claims.MaxTeams },
		func(e license.Entitlement, current int) bool { return e.AllowsTeams(current) },
		func(c setupCounts) int { return c.Teams },
	)
}
