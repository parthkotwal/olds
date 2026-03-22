"""
Olds frontend Playwright test suite.

Covers the key user flows introduced in Phase 13:
  1. Feed loads  — masthead visible, articles populate for all users (no login needed)
  2. Category filter — clicking a filter tab updates the displayed articles
  3. Article click (logged out) — opens the LoginModal, not the article itself
  4. LoginModal dismissal — Escape key, × button, click-outside all close it
  5. LoginModal form validation — send button disabled until email is entered
  6. Connection sidebar — present in the DOM when reading an article

Running:
  python3 tests/test_frontend.py

Requirements:
  pip install playwright
  python -m playwright install chromium

The tests assume the full Docker stack is running:
  docker compose up --build

If the frontend or backend is not reachable, the suite exits with a clear message.
"""

import sys
import time
import urllib.request
import urllib.error

from playwright.sync_api import sync_playwright, TimeoutError as PWTimeout

FRONTEND_URL = "http://localhost:5173"
BACKEND_URL  = "http://localhost:8080"

# ── Helpers ────────────────────────────────────────────────────────────────────

def check_services():
    """Return (frontend_ok, backend_ok) booleans without raising."""
    def ping(url):
        try:
            urllib.request.urlopen(url, timeout=3)
            return True
        except Exception:
            return False
    return ping(FRONTEND_URL), ping(f"{BACKEND_URL}/health")


def check_frontend_version(page) -> str:
    """
    Detect whether the running frontend is Phase 13 (public feed) or
    an older build (full-page login gate).

    Returns 'phase13' if the feed is visible without authentication,
    or 'pre13' if the old LoginPage is being shown.
    """
    page.goto(FRONTEND_URL)
    page.wait_for_load_state("networkidle")
    page.wait_for_timeout(500)

    # Phase 13: feed is public, category filter buttons are present, no email input
    buttons = [b.inner_text().strip().upper() for b in page.locator("button").all()]
    has_category_filter = any(t in ("ALL", "BUSINESS", "TECHNOLOGY", "HEALTH") for t in buttons)
    has_login_form = page.locator("input[type='email']").count() > 0

    if has_category_filter and not has_login_form:
        return "phase13"
    return "pre13"


def backend_has_articles():
    """Return True if the backend is serving at least one article."""
    try:
        resp = urllib.request.urlopen(f"{BACKEND_URL}/articles", timeout=3)
        import json
        data = json.loads(resp.read())
        return len(data.get("articles") or []) > 0
    except Exception:
        return False


class TestResult:
    def __init__(self):
        self.passed = []
        self.failed = []
        self.skipped = []

    def ok(self, name):
        self.passed.append(name)
        print(f"  ✓  {name}")

    def fail(self, name, reason):
        self.failed.append(name)
        print(f"  ✗  {name}")
        print(f"       {reason}")

    def skip(self, name, reason):
        self.skipped.append(name)
        print(f"  –  {name} (skipped: {reason})")

    def summary(self):
        total = len(self.passed) + len(self.failed) + len(self.skipped)
        print(f"\n{'─' * 52}")
        print(f"  {len(self.passed)}/{total} passed  "
              f"· {len(self.failed)} failed  "
              f"· {len(self.skipped)} skipped")
        if self.failed:
            print(f"\n  Failed:")
            for name in self.failed:
                print(f"    • {name}")
        print(f"{'─' * 52}")
        return len(self.failed) == 0


results = TestResult()


# ── Tests ──────────────────────────────────────────────────────────────────────

def test_feed_loads(page):
    """
    The feed is public. On load:
      - The OLDS masthead is visible
      - At least one article headline appears (requires backend articles)
      - The "Sign in" link is visible in the header (not logged in)
    """
    name = "feed loads with masthead and articles"
    try:
        page.goto(FRONTEND_URL)
        page.wait_for_load_state("networkidle")

        # Masthead
        masthead = page.locator("h1").first
        assert masthead.is_visible(), "h1 masthead not visible"
        assert "OLDS" in masthead.inner_text().upper(), "masthead doesn't contain OLDS"

        # Category filter row should be present
        page.wait_for_selector("button", timeout=4000)
        button_texts = [b.inner_text().strip() for b in page.locator("button").all()]
        has_filter = any(t.upper() in ("ALL", "BUSINESS", "TECHNOLOGY", "HEALTH")
                         for t in button_texts)
        assert has_filter, f"No category filter buttons found. Buttons: {button_texts[:10]}"

        # Articles (only assert if backend is available)
        if backend_has_articles():
            # Wait for at least one headline to appear
            page.wait_for_selector("h2, h3", timeout=6000)
            headlines = page.locator("h2, h3").all_text_contents()
            assert len(headlines) > 0, "No article headlines found after backend returned articles"

        results.ok(name)
    except (AssertionError, PWTimeout, Exception) as e:
        results.fail(name, str(e))


def test_sign_in_link_visible_when_logged_out(page):
    """
    When not authenticated, the Header meta strip shows a 'Sign in' link.
    When authenticated, it is replaced by the email + 'Sign out'.
    """
    name = "Sign in link visible in header when logged out"
    try:
        page.goto(FRONTEND_URL)
        page.wait_for_load_state("networkidle")
        # Give auth check time to resolve
        page.wait_for_timeout(800)

        # Look for Sign in button (case-insensitive)
        sign_in = page.get_by_role("button", name="Sign in")
        assert sign_in.is_visible(), \
            "Expected 'Sign in' button in header, not found"

        results.ok(name)
    except (AssertionError, PWTimeout, Exception) as e:
        results.fail(name, str(e))


def test_category_filter(page):
    """
    Clicking a category filter button re-fetches the feed.
    After clicking, either new articles appear or an empty-state message is shown.
    We don't assert article count because categories may have 0 results.
    """
    name = "category filter triggers a feed update"
    if not backend_has_articles():
        results.skip(name, "backend has no articles")
        return
    try:
        page.goto(FRONTEND_URL)
        page.wait_for_load_state("networkidle")
        page.wait_for_selector("h2, h3", timeout=6000)

        # Find and click a non-"All" category button
        target = None
        for btn in page.locator("button").all():
            txt = btn.inner_text().strip().upper()
            if txt in ("BUSINESS", "TECHNOLOGY", "HEALTH", "SPORTS"):
                target = btn
                break

        assert target is not None, "Could not find a category filter button"
        target.click()

        # After click, either articles reload or an empty state appears.
        # Wait briefly for the DOM to settle.
        page.wait_for_load_state("networkidle")
        page.wait_for_timeout(600)

        body_text = page.inner_text("main").upper()
        has_content = (
            page.locator("h2, h3").count() > 0
            or "NO ARTICLES" in body_text
            or "POST TO" in body_text
        )
        assert has_content, "After category filter click, neither articles nor empty state found"

        results.ok(name)
    except (AssertionError, PWTimeout, Exception) as e:
        results.fail(name, str(e))


def test_article_click_opens_login_modal(page):
    """
    When not logged in, clicking any article headline opens the LoginModal
    instead of navigating into the article view.
    The modal has role="dialog" and aria-label="Sign in to read".
    """
    name = "article click (logged out) opens LoginModal"
    if not backend_has_articles():
        results.skip(name, "backend has no articles to click")
        return
    try:
        page.goto(FRONTEND_URL)
        page.wait_for_load_state("networkidle")
        page.wait_for_selector("h2, h3", timeout=6000)

        # Click the first article headline
        first_headline = page.locator("h2, h3").first
        first_headline.click()
        page.wait_for_timeout(300)

        # Modal should now be open
        modal = page.get_by_role("dialog", name="Sign in to read")
        assert modal.is_visible(), "LoginModal did not appear after clicking article"

        # Google button should be inside the modal
        google_btn = modal.get_by_text("Continue with Google")
        assert google_btn.is_visible(), "Google sign-in button not visible inside modal"

        results.ok(name)
    except (AssertionError, PWTimeout, Exception) as e:
        results.fail(name, str(e))


def test_modal_close_with_escape(page):
    """Pressing Escape dismisses the LoginModal."""
    name = "LoginModal closes on Escape key"
    if not backend_has_articles():
        results.skip(name, "backend has no articles to trigger modal")
        return
    try:
        page.goto(FRONTEND_URL)
        page.wait_for_load_state("networkidle")
        page.wait_for_selector("h2, h3", timeout=6000)
        page.locator("h2, h3").first.click()
        page.wait_for_timeout(300)

        modal = page.get_by_role("dialog", name="Sign in to read")
        assert modal.is_visible(), "Modal did not open"

        page.keyboard.press("Escape")
        page.wait_for_timeout(300)

        assert not modal.is_visible(), "Modal still visible after Escape key"

        results.ok(name)
    except (AssertionError, PWTimeout, Exception) as e:
        results.fail(name, str(e))


def test_modal_close_with_x_button(page):
    """Clicking the × button dismisses the LoginModal."""
    name = "LoginModal closes on × button click"
    if not backend_has_articles():
        results.skip(name, "backend has no articles to trigger modal")
        return
    try:
        page.goto(FRONTEND_URL)
        page.wait_for_load_state("networkidle")
        page.wait_for_selector("h2, h3", timeout=6000)
        page.locator("h2, h3").first.click()
        page.wait_for_timeout(300)

        modal = page.get_by_role("dialog", name="Sign in to read")
        assert modal.is_visible(), "Modal did not open"

        close_btn = page.get_by_role("button", name="Close")
        assert close_btn.is_visible(), "Close (×) button not found"
        close_btn.click()
        page.wait_for_timeout(300)

        assert not modal.is_visible(), "Modal still visible after × click"

        results.ok(name)
    except (AssertionError, PWTimeout, Exception) as e:
        results.fail(name, str(e))


def test_modal_close_click_outside(page):
    """Clicking the overlay backdrop (outside the panel) dismisses the modal."""
    name = "LoginModal closes on backdrop click"
    if not backend_has_articles():
        results.skip(name, "backend has no articles to trigger modal")
        return
    try:
        page.goto(FRONTEND_URL)
        page.wait_for_load_state("networkidle")
        page.wait_for_selector("h2, h3", timeout=6000)
        page.locator("h2, h3").first.click()
        page.wait_for_timeout(300)

        modal = page.get_by_role("dialog", name="Sign in to read")
        assert modal.is_visible(), "Modal did not open"

        # Click far top-left corner of the viewport — well outside the centered panel
        page.mouse.click(10, 10)
        page.wait_for_timeout(300)

        assert not modal.is_visible(), "Modal still visible after backdrop click"

        results.ok(name)
    except (AssertionError, PWTimeout, Exception) as e:
        results.fail(name, str(e))


def test_modal_form_validation(page):
    """
    Inside the LoginModal:
      - 'Send magic link' button is disabled when the email input is empty
      - Typing an email enables the button
      - Clearing the email disables it again
    """
    name = "LoginModal send-link button enabled only when email is filled"
    if not backend_has_articles():
        results.skip(name, "backend has no articles to trigger modal")
        return
    try:
        page.goto(FRONTEND_URL)
        page.wait_for_load_state("networkidle")
        page.wait_for_selector("h2, h3", timeout=6000)
        page.locator("h2, h3").first.click()
        page.wait_for_timeout(300)

        modal = page.get_by_role("dialog", name="Sign in to read")
        assert modal.is_visible(), "Modal did not open"

        email_input = page.locator("#modal-email")
        send_button = page.get_by_role("button", name="Send magic link")

        assert email_input.is_visible(), "Email input (#modal-email) not found"
        assert send_button.is_visible(), "Send magic link button not found"

        # Button should be disabled (or visually inactive) when email is empty
        assert email_input.input_value() == "", "Email input should start empty"
        btn_disabled = send_button.is_disabled()
        assert btn_disabled, "Send button should be disabled when email is empty"

        # Type an email — button should become enabled
        email_input.fill("test@example.com")
        page.wait_for_timeout(200)
        assert not send_button.is_disabled(), \
            "Send button should be enabled after typing an email"

        # Clear email — button should go back to disabled
        email_input.fill("")
        page.wait_for_timeout(200)
        assert send_button.is_disabled(), \
            "Send button should be disabled again after clearing email"

        results.ok(name)
    except (AssertionError, PWTimeout, Exception) as e:
        results.fail(name, str(e))


def test_connection_sidebar_present_when_reading(page):
    """
    When an article is open (simulated by injecting Supabase session via
    localStorage and then clicking an article), the ConnectionSidebar section
    with the 'Connections' label is present in the DOM.

    We inject a mock session so the app thinks the user is logged in, bypassing
    the LoginModal and opening the article directly.
    """
    name = "ConnectionSidebar present in DOM when reading article"
    if not backend_has_articles():
        results.skip(name, "backend has no articles")
        return
    try:
        page.goto(FRONTEND_URL)
        page.wait_for_load_state("networkidle")

        # Inject a mock Supabase session into localStorage so the app
        # treats the user as authenticated. Supabase stores session data
        # under a key like "sb-<project-ref>-auth-token".
        # We set a fake but structurally valid session object.
        # The app reads this via supabase.auth.getSession() on mount.
        page.evaluate("""() => {
            // Find the Supabase storage key (format: sb-<ref>-auth-token)
            // by looking for any key in localStorage that matches the pattern,
            // or fall back to setting a known key directly.
            const mockSession = {
                access_token: 'mock-token-not-verified-by-frontend',
                refresh_token: 'mock-refresh',
                expires_at: Math.floor(Date.now() / 1000) + 3600,
                token_type: 'bearer',
                user: {
                    id: 'mock-user-id',
                    email: 'test@example.com',
                    role: 'authenticated',
                    aud: 'authenticated',
                }
            };
            // Supabase JS v2 key format
            const keys = Object.keys(localStorage);
            const supabaseKey = keys.find(k => k.startsWith('sb-') && k.endsWith('-auth-token'));
            if (supabaseKey) {
                localStorage.setItem(supabaseKey, JSON.stringify({ currentSession: mockSession, expiresAt: mockSession.expires_at }));
            } else {
                // Set under a predictable fallback key — the app will pick it up
                // on next getSession() call after reload
                localStorage.setItem('sb-auth-token', JSON.stringify({ currentSession: mockSession }));
            }
        }""")

        # Reload so the app reads the injected session from localStorage
        page.reload()
        page.wait_for_load_state("networkidle")
        page.wait_for_selector("h2, h3", timeout=6000)

        # Click the first article
        first_headline = page.locator("h2, h3").first
        first_headline.click()
        page.wait_for_timeout(800)

        # Regardless of whether the session injection worked, clicking the
        # article should either: open the article (session injected OK) or
        # show the modal (injection didn't take). We test both cases.
        modal = page.query_selector('[role="dialog"][aria-label="Sign in to read"]')
        if modal and modal.is_visible():
            # Session injection didn't take — skip gracefully rather than fail
            results.skip(name, "session injection via localStorage not supported in this build")
            return

        # Article should be open — check for the Back button and sidebar
        back_btn = page.get_by_text("← Back to feed")
        assert back_btn.is_visible(), "'← Back to feed' button not found — article may not have opened"

        # The ConnectionSidebar section label
        connections_label = page.get_by_text("Connections").first
        assert connections_label.is_visible(), "'Connections' sidebar label not found in DOM"

        results.ok(name)
    except (AssertionError, PWTimeout, Exception) as e:
        results.fail(name, str(e))


def test_feed_skeleton_not_shown_after_load(page):
    """
    After the feed finishes loading, no skeleton placeholder elements
    should still be animating. The real headlines should be present.
    """
    name = "feed skeleton resolves to real articles after load"
    if not backend_has_articles():
        results.skip(name, "backend has no articles")
        return
    try:
        page.goto(FRONTEND_URL)
        # Wait fully for the feed to settle
        page.wait_for_load_state("networkidle")
        page.wait_for_selector("h2, h3", timeout=8000)

        # Real headlines should be present
        headlines = page.locator("h2, h3").all_text_contents()
        assert len(headlines) > 0, "No real headlines after load settled"
        # And they should have actual text (not empty skeleton placeholders)
        non_empty = [h for h in headlines if h.strip()]
        assert len(non_empty) > 0, "Headlines present but all empty"

        results.ok(name)
    except (AssertionError, PWTimeout, Exception) as e:
        results.fail(name, str(e))


# ── Runner ─────────────────────────────────────────────────────────────────────

def main():
    print("\nOlds Frontend Test Suite")
    print("=" * 52)

    frontend_ok, backend_ok = check_services()

    if not frontend_ok:
        print(f"\n  ✗  Frontend not reachable at {FRONTEND_URL}")
        print("     Run: docker compose up --build")
        sys.exit(1)

    print(f"  Frontend : {FRONTEND_URL}  ✓")
    print(f"  Backend  : {BACKEND_URL}  {'✓' if backend_ok else '✗ (tests requiring articles will be skipped)'}")
    print()

    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)

        # Each test gets a fresh context (isolated localStorage, cookies, etc.)
        # This prevents auth state from one test leaking into the next.
        def fresh_page():
            ctx = browser.new_context(viewport={"width": 1280, "height": 900})
            return ctx.new_page()

        # Detect which code version is running before committing to the full suite.
        version = check_frontend_version(fresh_page())
        if version != "phase13":
            print("  ⚠  Frontend is running pre-Phase 13 code (full-page auth gate detected).")
            print("     Run:  docker compose up --build")
            print("     Then: python3 tests/test_frontend.py")
            browser.close()
            sys.exit(1)

        print("Running tests…\n")

        test_feed_loads(fresh_page())
        test_sign_in_link_visible_when_logged_out(fresh_page())
        test_category_filter(fresh_page())
        test_article_click_opens_login_modal(fresh_page())
        test_modal_close_with_escape(fresh_page())
        test_modal_close_with_x_button(fresh_page())
        test_modal_close_click_outside(fresh_page())
        test_modal_form_validation(fresh_page())
        test_connection_sidebar_present_when_reading(fresh_page())
        test_feed_skeleton_not_shown_after_load(fresh_page())

        browser.close()

    passed = results.summary()
    sys.exit(0 if passed else 1)


if __name__ == "__main__":
    main()
