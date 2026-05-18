package thtr

import (
	"strings"
	"testing"
)

func TestFormatCanonicalizesDocumentAndPreservesComments(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
# heading
http
  session browser = http.session.browser()
scenario login(email: string!)
  # act comment
  act submit
    do action.generate
      outputs:
        profile_id: $response_json | path("/id")
    expect otp: (field(body)|decode(json)|path("/data")) matches r"^[A-Z0-9]{6}$"
call run = login(email: "user@example.test") # call comment
`)

	formatted, err := Format(source)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}

	const want = `stage smoke

# heading
http
  session browser = http.session.browser()

scenario login(email: string!)
  # act comment
  act submit
    do action.generate
      outputs:
        profile_id: $response_json | path("/id")
    expect otp: (
      field(body)
      | decode(json)
      | path("/data")
    ) matches r"^[A-Z0-9]{6}$"

call run = login(email: "user@example.test") # call comment
`

	if got := string(formatted); got != want {
		t.Fatalf("formatted output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}

	reformatted, err := Format(formatted)
	if err != nil {
		t.Fatalf("reformat failed: %v", err)
	}
	if got := string(reformatted); got != want {
		t.Fatalf("format must be idempotent:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatRendersScenarioPreflight(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario send-email(recipient_email: string!, allow_non_test_recipient: bool)
  preflight recipient-test-domain: $recipient_email matches r"^[^@]+@example\.test$" override $allow_non_test_recipient
  act send
    do action.send()
call run = send-email(recipient_email: "person@example.test")
`)

	formatted, err := Format(source)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}

	const want = `stage smoke

scenario send-email(recipient_email: string!, allow_non_test_recipient: bool)
  preflight recipient-test-domain: $recipient_email matches r"^[^@]+@example\.test$" override $allow_non_test_recipient
  act send
    do action.send()

call run = send-email(recipient_email: "person@example.test")
`

	if got := string(formatted); got != want {
		t.Fatalf("formatted mismatch:\n%s", got)
	}
}

func TestFormatCanonicalizesActLogSugar(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    do action.http(method:"GET",url:"/login")
    log response=object{status:field(status_code),user_id:field(body)|decode(json)|path("/data/id")}
`)

	formatted, err := Format(source)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}

	const want = `stage smoke

scenario login
  act submit
    do action.http(method: "GET", url: "/login")
    log response = object {
      status: field(status_code),
      user_id: field(body) | decode(json) | path("/data/id")
    }
`

	if got := string(formatted); got != want {
		t.Fatalf("formatted output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatCanonicalizesStateAliasEntries(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
state
 record shared_meta = state.record(backend: local,record: "env/shared-meta", min_guarantee: local-atomic)
 record shared_meta_block = state.record
  backend: local
  record: "env/shared-meta-block"
  min_guarantee: local-atomic
 pool otp_identities_inline = state.pool(backend: local, pool: "otp-identities-inline", min_guarantee: local-atomic)
 pool otp_identities = state.pool
  backend: local
  pool: "otp-identities"
  min_guarantee: local-atomic
scenario login
 act submit
  do action.http(method: "GET")
`)

	formatted, err := Format(source)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}

	const want = `stage smoke

state
  record shared_meta = state.record
    backend: local
    record: "env/shared-meta"
    min_guarantee: local-atomic
  record shared_meta_block = state.record
    backend: local
    record: "env/shared-meta-block"
    min_guarantee: local-atomic
  pool otp_identities_inline = state.pool
    backend: local
    pool: "otp-identities-inline"
    min_guarantee: local-atomic
  pool otp_identities = state.pool
    backend: local
    pool: "otp-identities"
    min_guarantee: local-atomic

scenario login
  act submit
    do action.http(method: "GET")
`

	if got := string(formatted); got != want {
		t.Fatalf("formatted output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatPreservesNestedCommentsInsideBlocksAndPipelines(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    do action.generate
      # outputs comment
      outputs:
        # profile comment
        profile_id: $response_json | path("/id") # trailing mapping comment
    expect otp: (
      # base comment
      field(body) # body comment
      # decode comment
      | decode(json)
    ) matches r"ok"
`)

	formatted, err := Format(source)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}

	const want = `stage smoke

scenario login
  act submit
    do action.generate
      # outputs comment
      outputs:
        # profile comment
        profile_id: $response_json | path("/id") # trailing mapping comment
    expect otp: (
      # base comment
      field(body) # body comment
      # decode comment
      | decode(json)
    ) matches r"ok"
`

	if got := string(formatted); got != want {
		t.Fatalf("formatted output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatPreservesCommentsInsideObjectAndListExpressions(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    do action.http
      json: object {
        email: $email,
        # csrf comment
        csrf: $csrf
      }
      headers: list [
        "x-a",
        # header comment
        "x-b"
      ]
`)

	formatted, err := Format(source)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}

	const want = `stage smoke

scenario login
  act submit
    do action.http
      json: object {
        email: $email,
        # csrf comment
        csrf: $csrf
      }
      headers: list [
        "x-a",
        # header comment
        "x-b"
      ]
`

	if got := string(formatted); got != want {
		t.Fatalf("formatted output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatPreservesReadableMultilineObjectInputs(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    do action.http
      method: "POST"
      url: "/users"
      json: object {
        email: $email,
        profile: object {
          display_name: "Demo User",
          timezone: "Europe/Vilnius"
        },
        roles: list [
          "admin",
          "support"
        ]
      }
`)

	formatted, err := Format(source)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}

	const want = `stage smoke

scenario login
  act submit
    do action.http
      method: "POST"
      url: "/users"
      json: object {
        email: $email,
        profile: object {
          display_name: "Demo User",
          timezone: "Europe/Vilnius"
        },
        roles: list [
          "admin",
          "support"
        ]
      }
`

	if got := string(formatted); got != want {
		t.Fatalf("formatted output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatBreaksLongExpectedObjectAssertions(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET", url: "/users/123")
    expect profile: field(body) | decode(json) == object { id: "usr_1234567890", email: "demo@example.test", display_name: "Demo User", timezone: "Europe/Vilnius" }
`)

	formatted, err := Format(source)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}

	const want = `stage smoke

scenario login
  act submit
    do action.http(method: "GET", url: "/users/123")
    expect profile: field(body) | decode(json) == object {
      id: "usr_1234567890",
      email: "demo@example.test",
      display_name: "Demo User",
      timezone: "Europe/Vilnius"
    }
`

	if got := string(formatted); got != want {
		t.Fatalf("formatted output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatBreaksLongExpectedListAssertions(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET", url: "/users/123")
    expect roles: field(body) | decode(json) | path("/roles") == list [ "administrator", "support-operator", "billing-reviewer", "read-only-auditor" ]
`)

	formatted, err := Format(source)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}

	const want = `stage smoke

scenario login
  act submit
    do action.http(method: "GET", url: "/users/123")
    expect roles: field(body) | decode(json) | path("/roles") == list [
      "administrator",
      "support-operator",
      "billing-reviewer",
      "read-only-auditor"
    ]
`

	if got := string(formatted); got != want {
		t.Fatalf("formatted output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatBreaksLongAssertCallObjectArguments(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET", url: "/users/123")
    expect profile: field(body) | decode(json) assert expectation.equal(expected: object { id: "usr_1234567890", email: "demo@example.test", display_name: "Demo User", timezone: "Europe/Vilnius" })
`)

	formatted, err := Format(source)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}

	const want = `stage smoke

scenario login
  act submit
    do action.http(method: "GET", url: "/users/123")
    expect profile: field(body) | decode(json) assert expectation.equal(expected: object {
      id: "usr_1234567890",
      email: "demo@example.test",
      display_name: "Demo User",
      timezone: "Europe/Vilnius"
    })
`

	if got := string(formatted); got != want {
		t.Fatalf("formatted output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatBreaksLongAssertCallListArguments(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET", url: "/users/123")
    expect roles: field(body) | decode(json) | path("/roles") assert expectation.equal(expected: list [ "administrator", "support-operator", "billing-reviewer", "read-only-auditor" ])
`)

	formatted, err := Format(source)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}

	const want = `stage smoke

scenario login
  act submit
    do action.http(method: "GET", url: "/users/123")
    expect roles: field(body) | decode(json) | path("/roles") assert expectation.equal(expected: list [
      "administrator",
      "support-operator",
      "billing-reviewer",
      "read-only-auditor"
    ])
`

	if got := string(formatted); got != want {
		t.Fatalf("formatted output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatRendersQuotedDataKeysOnlyWhenNeeded(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    do action.http
      json: object { "email": $email, x-csrf-token: $csrf, "Content-Type": "application/json", "profile.name": "Demo", "@type": "User" }
`)

	formatted, err := Format(source)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}

	const want = `stage smoke

scenario login
  act submit
    do action.http
      json: object {
        email: $email,
        x-csrf-token: $csrf,
        Content-Type: "application/json",
        "profile.name": "Demo",
        "@type": "User"
      }
`

	if got := string(formatted); got != want {
		t.Fatalf("formatted output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatRendersMultilineQuotedDataKeys(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    do action.http
      json: object {
        "email": $email,
        # profile comment
        "profile.name": "Demo",
        x-csrf-token: $csrf
      }
`)

	formatted, err := Format(source)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}

	const want = `stage smoke

scenario login
  act submit
    do action.http
      json: object {
        email: $email,
        # profile comment
        "profile.name": "Demo",
        x-csrf-token: $csrf
      }
`

	if got := string(formatted); got != want {
		t.Fatalf("formatted output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}

	reformatted, err := Format(formatted)
	if err != nil {
		t.Fatalf("reformat failed: %v", err)
	}
	if got := string(reformatted); got != want {
		t.Fatalf("format must be idempotent:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatPreservesCommentsInsideScenarioCallBindings(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login(email: string!)
  act submit
    do action.http(method: "GET", url: "/health")

call run = login(
  # email comment
  email: "user@example.test", # trailing binding comment
)
`)

	formatted, err := Format(source)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}

	const want = `stage smoke

scenario login(email: string!)
  act submit
    do action.http(method: "GET", url: "/health")

call run = login(
  # email comment
  email: "user@example.test" # trailing binding comment
)
`

	if got := string(formatted); got != want {
		t.Fatalf("formatted output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatCanonicalizesNamesDependenciesAndCaptureAuth(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
name "Smoke stage"
scenario auth/login
 name "Login scenario"
 act submit
  name "Submit request"
  do action.http(method: "POST", url: "/login")
  capture_auth web
   csrf: response_header("X-CSRF-Token")
   session: response_cookie("session")
call run-login = auth/login()
 name "Run login"
 dependency bootstrap
 dependency provision-user when done
`)

	formatted, err := Format(source)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}

	const want = `stage smoke
name "Smoke stage"

scenario auth/login
  name "Login scenario"
  act submit
    name "Submit request"
    do action.http(method: "POST", url: "/login")
    capture_auth web
      csrf: response_header("X-CSRF-Token")
      session: response_cookie("session")

call run-login = auth/login()
  name "Run login"
  dependency bootstrap
  dependency provision-user when done
`

	if got := string(formatted); got != want {
		t.Fatalf("formatted output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatKeepsGroupedPipelineContainerIndentStable(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET", url: "/health")
    expect payload: (
      object {
        email: $email,
        # csrf comment
        csrf: $csrf
      }
      | path("/csrf")
    ) matches r"ok"
`)

	formatted, err := Format(source)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}

	const want = `stage smoke

scenario login
  act submit
    do action.http(method: "GET", url: "/health")
    expect payload: (
      object {
        email: $email,
        # csrf comment
        csrf: $csrf
      }
      | path("/csrf")
    ) matches r"ok"
`

	if got := string(formatted); got != want {
		t.Fatalf("formatted output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatKeepsCommentBetweenScenarioCallHeaderAndExportsOutsideBindingBlock(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login(email: string!)
  act submit
    do action.http(method: "GET", url: "/health")
    export issued = field(body)

call run = login(
  email: "user@example.test"
)
# between header and export
  export token = $issued
`)

	formatted, err := Format(source)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}

	const want = `stage smoke

scenario login(email: string!)
  act submit
    do action.http(method: "GET", url: "/health")
    export issued = field(body)

call run = login(email: "user@example.test")
# between header and export
  export token = $issued
`

	if got := string(formatted); got != want {
		t.Fatalf("formatted output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatKeepsInlineScenarioCallBindingsInlineWhenOnlyExportBoundaryCommentExists(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login(email: string!)
  act submit
    do action.http(method: "GET", url: "/health")
    export issued = field(body)

call run = login(email: "user@example.test")
# between header and export
  export token = $issued
`)

	formatted, err := Format(source)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}

	const want = `stage smoke

scenario login(email: string!)
  act submit
    do action.http(method: "GET", url: "/health")
    export issued = field(body)

call run = login(email: "user@example.test")
# between header and export
  export token = $issued
`

	if got := string(formatted); got != want {
		t.Fatalf("formatted output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatKeepsIndentedExportBoundaryCommentOutsideInlineScenarioCallHeader(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login(email: string!)
  act submit
    do action.http(method: "GET", url: "/health")
    export issued = field(body)

call run = login(email: "user@example.test")
  # comment before export at export indent
  export token = $issued
`)

	formatted, err := Format(source)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}

	const want = `stage smoke

scenario login(email: string!)
  act submit
    do action.http(method: "GET", url: "/health")
    export issued = field(body)

call run = login(email: "user@example.test")
  # comment before export at export indent
  export token = $issued
`

	if got := string(formatted); got != want {
		t.Fatalf("formatted output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatKeepsCommentsInsideNonPipelineGroupedExpressions(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET", url: "/health")
    expect status: (
      field(status_code)
      # grouped comment
    ) == 200
`)

	formatted, err := Format(source)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}

	const want = `stage smoke

scenario login
  act submit
    do action.http(method: "GET", url: "/health")
    expect status: (
      field(status_code)
      # grouped comment
    ) == 200
`

	if got := string(formatted); got != want {
		t.Fatalf("formatted output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatRendersScalarAndUnaryExpectationSugar(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET", url: "/health")
    expect page-text: field(body)contains"Example Domain"
    expect latency-high: field(duration_ms)>100
    expect retries-ok: field(retry_count)<=10
    expect not-server-error: field(status_code)not>=500
    expect has-trace: field(headers)has key("X-Trace")
`)

	formatted, err := Format(source)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}

	const want = `stage smoke

scenario login
  act submit
    do action.http(method: "GET", url: "/health")
    expect page-text: field(body) contains "Example Domain"
    expect latency-high: field(duration_ms) > 100
    expect retries-ok: field(retry_count) <= 10
    expect not-server-error: field(status_code) not >= 500
    expect has-trace: field(headers) has key("X-Trace")
`

	if got := string(formatted); got != want {
		t.Fatalf("formatted output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatRendersNegatedAssertCall(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET", url: "/health")
    expect not-custom-error: field(status_code)not assert plugin.custom(expected:500)
`)

	formatted, err := Format(source)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}

	const want = `stage smoke

scenario login
  act submit
    do action.http(method: "GET", url: "/health")
    expect not-custom-error: field(status_code) not assert plugin.custom(expected: 500)
`

	if got := string(formatted); got != want {
		t.Fatalf("formatted output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatRendersPresenceAndNullAssertions(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    do action.http(method:"GET",url:"/health")
    expect not-not-found: field(status_code)!=404
    expect no-error-key: field(body)|decode(json)lacks key("error")
    expect no-warning-key: field(body)|decode(json)has no key("warning")
    expect deleted-null: field(body)|decode(json)|path("/deleted_at")is null
    expect trace-present: field(body)|decode(json)|path("/trace_id")is present
    expect name-not-null: field(body)|decode(json)|path("/name")is not null
`)

	formatted, err := Format(source)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}

	const want = `stage smoke

scenario login
  act submit
    do action.http(method: "GET", url: "/health")
    expect not-not-found: field(status_code) != 404
    expect no-error-key: field(body) | decode(json) lacks key("error")
    expect no-warning-key: field(body) | decode(json) lacks key("warning")
    expect deleted-null: field(body) | decode(json) | path("/deleted_at") is null
    expect trace-present: field(body) | decode(json) | path("/trace_id") is present
    expect name-not-null: field(body) | decode(json) | path("/name") is not null
`

	if got := string(formatted); got != want {
		t.Fatalf("formatted output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatRendersBetweenExpectationSugar(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET", url: "/health")
    expect retries-range: field(retry_count)between 1 and 5
    expect not-retries-range: field(retry_count)not between 1 and 5
`)

	formatted, err := Format(source)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}

	const want = `stage smoke

scenario login
  act submit
    do action.http(method: "GET", url: "/health")
    expect retries-range: field(retry_count) between 1 and 5
    expect not-retries-range: field(retry_count) not between 1 and 5
`

	if got := string(formatted); got != want {
		t.Fatalf("formatted output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatRendersCanonicalBetweenAssertCall(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET", url: "/health")
    expect retries-range: field(retry_count)assert expectation.between(min:1,max:5)
    expect not-retries-range: field(retry_count)not assert expectation.between(min:1,max:5)
`)

	formatted, err := Format(source)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}

	const want = `stage smoke

scenario login
  act submit
    do action.http(method: "GET", url: "/health")
    expect retries-range: field(retry_count) assert expectation.between(min: 1, max: 5)
    expect not-retries-range: field(retry_count) not assert expectation.between(min: 1, max: 5)
`

	if got := string(formatted); got != want {
		t.Fatalf("formatted output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatRendersCollectionWhereExpectationSugar(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET", url: "/health")
    expect has-demo-notification: field(body)|decode(json)|path("/notifications") has item where path("/receiverAddress")=="demo@example.test"
    expect all-recipients-present: field(body)|decode(json)|path("/notifications") all items where (
      path("/receiverAddress")contains"@example.test",
      path("/subject")not assert plugin.custom(expected:"Verification Code")
    )
    expect active-user: field(body)|decode(json)has entry("status")=="active"
    expect external-status: field(body)|decode(json)has entry("status")assert matcher.smoke.equal(expected:"active")
`)

	formatted, err := Format(source)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}

	const want = `stage smoke

scenario login
  act submit
    do action.http(method: "GET", url: "/health")
    expect has-demo-notification: field(body) | decode(json) | path("/notifications") has item where path("/receiverAddress") == "demo@example.test"
    expect all-recipients-present: field(body) | decode(json) | path("/notifications") all items where (
      path("/receiverAddress") contains "@example.test",
      path("/subject") not assert plugin.custom(expected: "Verification Code")
    )
    expect active-user: field(body) | decode(json) has entry("status") == "active"
    expect external-status: field(body) | decode(json) has entry("status") assert matcher.smoke.equal(expected: "active")
`

	if got := string(formatted); got != want {
		t.Fatalf("formatted output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatRendersPickWhereSelector(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login(email: string!)
  act submit
    do action.http(method: "GET", url: "/notifications")
    export otp = (
      field(body)|decode(json)|path("/items")|pick where (
        path("/receiverAddress")==$email,
        path("/subject")contains"Verification"
      )|path("/body")
    )
`)

	formatted, err := Format(source)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}

	const want = `stage smoke

scenario login(email: string!)
  act submit
    do action.http(method: "GET", url: "/notifications")
    export otp = (
      field(body)
      | decode(json)
      | path("/items")
      | pick where (
        path("/receiverAddress") == $email,
        path("/subject") contains "Verification"
      )
      | path("/body")
    )
`

	if got := string(formatted); got != want {
		t.Fatalf("formatted output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatRendersActExportAssertion(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke
scenario login(email: string!)
  act submit
    do action.http(method:"GET",url:"/notifications")
    export otp = (
      field(body)|decode(json)|path("/items")|pick(at:"/receiverAddress",equals:$email)|path("/body")
    )matches r"^[0-9]{6}$"
`)

	formatted, err := Format(source)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}

	const want = `stage smoke

scenario login(email: string!)
  act submit
    do action.http(method: "GET", url: "/notifications")
    export otp = (
      field(body)
      | decode(json)
      | path("/items")
      | pick(at: "/receiverAddress", equals: $email)
      | path("/body")
    ) matches r"^[0-9]{6}$"
`

	if got := string(formatted); got != want {
		t.Fatalf("formatted output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatRejectsScenarioCallExportAssertion(t *testing.T) {
	t.Parallel()

	_, err := Format([]byte(`stage smoke
scenario login
  act submit
    do action.http(method: "GET", url: "/login")

call run = login()
  export token = $token matches r"^sess_"
`))
	if err == nil {
		t.Fatal("expected format to fail")
	}
	if !strings.Contains(err.Error(), "scenario call export assertions are not supported") {
		t.Fatalf("error mismatch: got %q", err.Error())
	}
}

func TestFormatIsIdempotent(t *testing.T) {
	t.Parallel()

	source := []byte(`stage smoke

scenario ping
  act get-health
    do action.http(method: "GET", url: "/health")
    expect status: field(status_code) == 200
`)

	first, err := Format(source)
	if err != nil {
		t.Fatalf("first format failed: %v", err)
	}

	second, err := Format(first)
	if err != nil {
		t.Fatalf("second format failed: %v", err)
	}

	if string(second) != string(first) {
		t.Fatalf("format must be idempotent:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}

func TestFormatReportsParseDiagnosticsWithSourceFile(t *testing.T) {
	t.Parallel()

	_, err := formatWithSource([]byte("stage smoke\nscenario\n"), "stage.thtr")
	if err == nil {
		t.Fatal("expected formatter diagnostic error, got nil")
	}

	diagnosticError, ok := err.(*DiagnosticError)
	if !ok {
		t.Fatalf("error type mismatch: got %T", err)
	}
	diagnostic := diagnosticError.Diagnostic()
	if got, want := diagnostic.Code, "thtr_parse_error"; got != want {
		t.Fatalf("diagnostic code mismatch: got %q want %q", got, want)
	}
	if got, want := diagnostic.Span.File, "stage.thtr"; got != want {
		t.Fatalf("diagnostic file mismatch: got %q want %q", got, want)
	}
	if !strings.Contains(diagnostic.Summary, "expected identifier") {
		t.Fatalf("diagnostic summary mismatch: got %q", diagnostic.Summary)
	}
}
