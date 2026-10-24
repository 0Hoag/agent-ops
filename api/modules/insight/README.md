# Delivery insights

`GET /api/v1/insights?repository_id=<uuid>` is a read-only, repository-filtered summary built only from retained, deduplicated webhook deliveries. It returns `send_back`, `first_ci_success`, `cycle_time`, and evidence-linked `suggestions`.

Send-back numerator is a recorded transition from `in_progress` to `review`; denominator is every recorded entry into `review`. A duplicate delivery cannot count twice because ingestion keys deliveries by provider/repository/delivery ID. First-run CI groups recorded pipeline events by SHA and uses the first recorded conclusion: success/pass is green, failure is not; runs without a conclusion are ineligible. This cannot infer unrecorded runs or distinguish a historical rerun when its SHA is absent.

Cycle time starts at the first recorded `in_progress` status and ends at the next recorded `done`. Reopens start a new interval; blocked time remains included; open intervals are excluded. The metric does not use `updated_at` and cannot reconstruct transitions missing from webhook history.

Suggestions require at least two recorded failure conclusions for the same SHA and include delivery evidence. They never write `FIVE-MINUTES.md`; one-off failures produce no suggestion. Empty or partial history is returned with `history.complete=false` and a reason.
