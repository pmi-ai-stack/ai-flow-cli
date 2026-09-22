package gitops

import "context"

// CheckoutBase switches dir's HEAD to base — used transiently by
// run-e2e to test against the base code before switching back.
func (g *GitOps) CheckoutBase(ctx context.Context, dir, base string) error {
	_, err := g.run(ctx, dir, "checkout", base)
	return err
}

// CheckoutBranch switches dir's HEAD back to branch.
func (g *GitOps) CheckoutBranch(ctx context.Context, dir, branch string) error {
	_, err := g.run(ctx, dir, "checkout", branch)
	return err
}
