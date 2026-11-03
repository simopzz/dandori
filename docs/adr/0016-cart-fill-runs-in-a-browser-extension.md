# Cart Fill runs in a browser extension

Cart Fill drives the supermarket's pages as the signed-in User. A hosted Dandori could only do that by holding the User's supermarket session or credentials, which breaks when the session expires and is likely against the supermarket's terms. Instead, Cart Fill runs in a Chromium extension in the User's own browser: it fetches the Grocery List from Dandori, fills the cart in the User's signed-in session, and reports what it added. Dandori never holds supermarket credentials. This extends ADR-0005: the extension also stops at the cart.
