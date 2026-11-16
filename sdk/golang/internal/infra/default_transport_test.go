package infra

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Kucoin/kucoin-universal-sdk/sdk/golang/pkg/generate/futures/market"
	"github.com/Kucoin/kucoin-universal-sdk/sdk/golang/pkg/generate/futures/positions"
	"github.com/Kucoin/kucoin-universal-sdk/sdk/golang/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultTransport_processPathVariable(t1 *testing.T) {
	t := DefaultTransport{}

	req := struct {
		Currency string `path:"currency"`
	}{Currency: "BTC"}

	result := t.processPathVariable("/test/{currency}", req)

	assert.Equal(t1, "/test/BTC", result)
}

func TestDefaultTransport_processPathVariable2(t1 *testing.T) {
	t := DefaultTransport{}

	req := struct {
		Currency string `path:"currency"`
	}{Currency: "BTC"}

	result := t.processPathVariable("/test/symbol", req)

	assert.Equal(t1, "/test/symbol", result)
}

func TestDefaultTransport_processPathVariable3(t1 *testing.T) {
	t := DefaultTransport{}

	var currency string = "BTC"
	req := struct {
		Currency *string `path:"currency"`
	}{Currency: &currency}

	result := t.processPathVariable("/test/{currency}", req)

	assert.Equal(t1, "/test/BTC", result)
}

type testRoundTripper func(*http.Request) (*http.Response, error)

func (f testRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestFuturesPositionAndSymbolPrecision(t *testing.T) {
	const long = `{
		"symbol":"XBTUSDTM", "positionSide":"LONG", "currentQty":9223372036854775808123,
		"avgEntryPrice":67430.123456789012345678901234567890,
		"markPrice":"67432.987654321098765432109876543210",
		"unrealisedPnl":-4.975000000000000000001E-7, "liquidationPrice":"45314.3300",
		"leverage":"2.88000000000000000001", "posMargin":22.50970458430000000001,
		"posCrossMargin":0.00000000000000000000001, "riskLimit":"500000.000000000000001",
		"posFunding":null, "openingTimestamp":1729155616322, "isOpen":true
	}`
	const short = `{"symbol":"XBTUSDTM", "positionSide":"SHORT", "currentQty":"-9223372036854775808123",
		"posCrossMargin":"0", "unrealisedPnl":"0"}`
	const multiplier = "1.234567890123456789000E-400"
	transport := NewDefaultTransport(&types.ClientOption{FuturesEndpoint: "https://test.invalid"}, "test")
	transport.httpClient.Transport = testRoundTripper(func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, http.MethodGet, req.Method)
		var data string
		switch req.URL.Path {
		case "/api/v1/position":
			assert.Equal(t, "XBTUSDTM", req.URL.Query().Get("symbol"))
			data = long
		case "/api/v1/positions":
			data = "[" + long + "," + short + "]"
		case "/api/v1/contracts/XBTUSDTM":
			data = `{"symbol":"XBTUSDTM", "multiplier":` + multiplier + `}`
		case "/api/v1/contracts/QUOTED":
			data = `{"symbol":"QUOTED", "multiplier":"` + multiplier + `"}`
		default:
			t.Fatalf("unexpected request path: %s", req.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Gw-Ratelimit-Remaining": {"123"}},
			Body:       io.NopCloser(strings.NewReader(`{"code":"200000", "data":` + data + `}`)),
		}, nil
	})
	ctx := context.Background()
	positionsAPI := positions.NewPositionsAPIImp(transport)
	details, err := positionsAPI.GetPositionDetails(positions.NewGetPositionDetailsReqBuilder().SetSymbol("XBTUSDTM").Build(), ctx)
	require.NoError(t, err)
	require.NotNil(t, details.CommonResponse)
	assert.Equal(t, types.CodeSuccess, details.CommonResponse.Code)
	assert.EqualValues(t, 123, details.CommonResponse.RateLimit.Remaining)
	assert.Equal(t, types.DecimalString("9223372036854775808123"), details.CurrentQty)
	assert.Equal(t, types.DecimalString("67430.123456789012345678901234567890"), details.AvgEntryPrice)
	assert.EqualValues(t, 1729155616322, details.OpeningTimestamp)
	assert.True(t, details.IsOpen)
	assert.Nil(t, details.PosFunding)

	list, err := positionsAPI.GetPositionList(positions.NewGetPositionListReqWithDefaults(), ctx)
	require.NoError(t, err)
	require.NotNil(t, list.CommonResponse)
	require.Len(t, list.Data, 2)
	assert.Equal(t, "LONG", list.Data[0].PositionSide)
	assert.Equal(t, "SHORT", list.Data[1].PositionSide)
	assert.Equal(t, types.DecimalString("-9223372036854775808123"), list.Data[1].CurrentQty)
	assert.Equal(t, "0", list.Data[1].ToMap()["posCrossMargin"])
	assert.Nil(t, list.Data[1].RiskLimit)
	assert.Nil(t, list.Data[1].ToMap()["riskLimit"])

	detailsJSON, err := json.Marshal(details)
	require.NoError(t, err)
	var detailsFields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(detailsJSON, &detailsFields))
	listJSON, err := json.Marshal(list)
	require.NoError(t, err)
	var listFields struct{ Data []map[string]json.RawMessage }
	require.NoError(t, json.Unmarshal(listJSON, &listFields))
	require.Len(t, listFields.Data, 2)
	assert.Equal(t, `"-9223372036854775808123"`, string(listFields.Data[1]["currentQty"]))
	assert.Equal(t, `"0"`, string(listFields.Data[1]["unrealisedPnl"]))
	assert.NotContains(t, listFields.Data[1], "riskLimit")
	detailsMap, listItemMap := details.ToMap(), list.Data[0].ToMap()
	for field, want := range map[string]string{
		"currentQty":       "9223372036854775808123",
		"avgEntryPrice":    "67430.123456789012345678901234567890",
		"markPrice":        "67432.987654321098765432109876543210",
		"unrealisedPnl":    "-4.975000000000000000001E-7",
		"liquidationPrice": "45314.3300",
		"leverage":         "2.88000000000000000001",
		"posMargin":        "22.50970458430000000001",
		"posCrossMargin":   "0.00000000000000000000001",
		"riskLimit":        "500000.000000000000001",
	} {
		assert.Equal(t, want, detailsMap[field], field)
		assert.Equal(t, want, listItemMap[field], field)
		assert.Equal(t, `"`+want+`"`, string(detailsFields[field]), field)
		assert.Equal(t, `"`+want+`"`, string(listFields.Data[0][field]), field)
	}

	marketAPI := market.NewMarketAPIImp(transport)
	for _, symbol := range []string{"XBTUSDTM", "QUOTED"} {
		resp, err := marketAPI.GetSymbol(market.NewGetSymbolReqBuilder().SetSymbol(symbol).Build(), ctx)
		require.NoError(t, err)
		require.NotNil(t, resp.CommonResponse)
		assert.Equal(t, types.DecimalString(multiplier), resp.Multiplier)
		assert.Equal(t, multiplier, resp.ToMap()["multiplier"])
		body, err := json.Marshal(resp)
		require.NoError(t, err)
		var decoded market.GetSymbolResp
		require.NoError(t, json.Unmarshal(body, &decoded))
		assert.Contains(t, string(body), `"multiplier":"`+multiplier+`"`)
		assert.Equal(t, resp.Multiplier, decoded.Multiplier)
	}
}

func TestFuturesDecimalEmptyAndInvalidResponses(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		err  string
	}{
		{"empty", `{"code":"200000","data":[]}`, ""},
		{"invalid price", `{"code":"200000","data":[{"markPrice":"NaN"}]}`, "failed to unmarshal response"},
		{"invalid quantity", `{"code":"200000","data":[{"currentQty":true}]}`, "failed to unmarshal response"},
		{"invalid margin", `{"code":"200000","data":[{"posCrossMargin":{}}]}`, "failed to unmarshal response"},
		{"server error", `{"code":"400100","msg":"invalid position request"}`, "invalid position request"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transport := NewDefaultTransport(&types.ClientOption{FuturesEndpoint: "https://test.invalid"}, "test")
			transport.httpClient.Transport = testRoundTripper(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			resp, err := positions.NewPositionsAPIImp(transport).GetPositionList(positions.NewGetPositionListReqWithDefaults(), context.Background())
			require.NotNil(t, resp.CommonResponse)
			if tc.err == "" {
				require.NoError(t, err)
				assert.Equal(t, []positions.GetPositionListData{}, resp.Data)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.err)
		})
	}

	for _, value := range []string{`""`, `"1e-"`, `"null"`, `[]`} {
		t.Run("invalid multiplier "+value, func(t *testing.T) {
			var resp market.GetSymbolResp
			require.Error(t, json.Unmarshal([]byte(`{"multiplier":`+value+`}`), &resp))
		})
	}
}

func TestFuturesPositionAndSymbolContextCancellation(t *testing.T) {
	transport := NewDefaultTransport(&types.ClientOption{
		FuturesEndpoint: "https://test.invalid",
		TransportOption: types.NewTransportOptionBuilder().SetMaxRetries(0).Build(),
	}, "test")
	positionsAPI := positions.NewPositionsAPIImp(transport)
	marketAPI := market.NewMarketAPIImp(transport)
	for _, tc := range []struct {
		name string
		call func(context.Context) error
	}{
		{"details", func(ctx context.Context) error {
			_, err := positionsAPI.GetPositionDetails(positions.NewGetPositionDetailsReqBuilder().SetSymbol("XBTUSDTM").Build(), ctx)
			return err
		}},
		{"list", func(ctx context.Context) error {
			_, err := positionsAPI.GetPositionList(positions.NewGetPositionListReqWithDefaults(), ctx)
			return err
		}},
		{"symbol", func(ctx context.Context) error {
			_, err := marketAPI.GetSymbol(market.NewGetSymbolReqBuilder().SetSymbol("XBTUSDTM").Build(), ctx)
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			transport.httpClient.Transport = testRoundTripper(func(req *http.Request) (*http.Response, error) {
				cancel()
				require.ErrorIs(t, req.Context().Err(), context.Canceled)
				return nil, req.Context().Err()
			})
			err := tc.call(ctx)
			require.Error(t, err)
			assert.Contains(t, err.Error(), context.Canceled.Error())
		})
	}
}
