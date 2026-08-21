# Milter endpoint for the mail core (V4 §39-40): Stalwart connects here
# during the SMTP DATA stage. self_scan makes the proxy scan in-process
# instead of forwarding to a separate scan worker.
milter = yes;
timeout = 120s;
upstream "local" {
  default = yes;
  self_scan = yes;
}
count = 1;
max_retries = 3;
discard_on_reject = false;
quarantine_on_reject = false;
spam_header = "X-Spam";
reject_message = "Message rejected by content policy";
