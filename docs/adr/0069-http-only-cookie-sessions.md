# HTTP-Only Cookie Sessions

The app uses local username and password accounts with HTTP-only cookie sessions for browser authentication. This fits the same-origin Caddy deployment better than browser-stored tokens, with password hashing and CSRF protection used where needed.

