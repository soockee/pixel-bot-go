---
title: "Sign in"
summary: "Sign in with Google; your account's role decides whether you may fish and upload recordings."
order: 2
---

Fishing with real input and uploading recordings need a Google account with a role; a dry run needs none.

1. Click **Sign in with Google** in the top bar. The browser opens Google sign-in; pick your account and return to the app.
2. The top bar shows your email and role. Without a role, **Start fishing** stays disabled: ask the owner to give your account a role that may fish, then sign out and in again.

The app keeps you signed in (`pixel_bot_account.bin`, encrypted for your Windows user) and checks your role on each start. **Sign out** forgets it and stops a real run. Release builds carry the service and sign-in client; a local build needs `cloud_url`, `oauth_client_id` and `oauth_client_secret` in the config. Without them the top bar has no **Sign in** button and fishing needs no role.
