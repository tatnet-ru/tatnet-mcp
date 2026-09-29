package tools

import (
	"context"
	"errors"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type BalanceIn struct{}

type BalanceOut struct {
	Currency        string `json:"currency"`
	Available       string `json:"available" jsonschema:"what paid resources can be bought with right now: balance plus bonus credits once they are unlocked. A decimal string"`
	Balance         string `json:"balance" jsonschema:"real money paid in and not yet spent; negative means debt"`
	Credits         string `json:"credits" jsonschema:"bonus balance (promo codes, referral grants)"`
	CreditsUnlocked bool   `json:"credits_unlocked" jsonschema:"bonus credits count towards payment only after the account's first real top-up"`
	RealTopupMin    string `json:"real_topup_min,omitempty" jsonschema:"smallest real top-up that unlocks the bonus credits; present only while they are locked"`
}

// balanceForbidden — 403 у баланса значит одно из двух, и общий совет
// «расширьте ключ» верен только для первого: видеть деньги может лишь
// владелец, а подключение по OAuth получает это право только при выдаче на
// весь аккаунт.
const balanceForbidden = "get balance failed (HTTP 403). Only the account owner can see the balance: an API key needs the billing:read action, and a connection authorized in the browser gets it only when the owner granted access to the whole account, not to selected projects. The user can reconnect as the owner with the whole account, or check the balance in the TatNet dashboard (Finances). Do not retry."

func (t *Tools) registerBalance(s *mcp.Server) {
	add(s, &mcp.Tool{
		Name:        "get_balance",
		Description: "Show the TatNet account balance: money available for paid resources right now, the real balance, bonus credits and whether they are spendable yet. Only the account owner can see it. Topping up is done in the TatNet dashboard, not here.",
		Annotations: readOnly("Account balance"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ BalanceIn) (*mcp.CallToolResult, BalanceOut, error) {
		var out BalanceOut
		c, err := t.client(ctx)
		if err != nil {
			return nil, out, err
		}
		r, err := c.AccountGetBalanceWithResponse(ctx)
		if err == nil && r.StatusCode() == http.StatusForbidden {
			return nil, out, errors.New(balanceForbidden)
		}
		// 503 billing_unavailable — «не смогли узнать», а не ноль: check
		// превращает его в отказ «повторите позже», и модель не скажет
		// человеку, что денег нет.
		if err := check("get balance", r, err); err != nil {
			return nil, out, err
		}
		b := r.JSON200
		out = BalanceOut{
			Currency:        b.Currency,
			Available:       b.Available,
			Balance:         b.Balance,
			Credits:         b.Credits,
			CreditsUnlocked: b.CreditsUnlocked,
		}
		if !b.CreditsUnlocked {
			out.RealTopupMin = b.RealTopupMin
		}
		return nil, out, nil
	})
}
