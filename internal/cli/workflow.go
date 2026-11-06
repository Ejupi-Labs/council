package cli

const workflowHelp = `
Verified workflow:
  council workflow preferences get
  council workflow preferences update --delivery review_only|apply_verified_local
    --expected-revision R [--request-id ID]
  council tasks start ... [--workflow auto|author_only] [--delivery MODE | --delivery-file FILE]
    [--checks-file FILE] [--review auto|required] [--max-corrections 0|1]
  council tasks workflow ID [--workflow-id ID | --task-version V]
  council tasks steps|reviews|checks|findings|deliveries|candidates ID
    [--workflow-id ID | --task-version V] [--limit N] [--cursor CURSOR]
  council candidates changes ID [--limit N] [--cursor CURSOR]
  council tasks workflow-adopt ID --expected-revision R --task-version V [workflow options]
  council tasks advance ID --expected-revision R --workflow-revision W --task-version V
  council tasks check ID --check-id ID --expected-revision R --workflow-revision W --task-version V

Omitted delivery captures the saved prospective preference. Local application authorizes no
commit, push or publication. External actions require an explicit scoped delivery-file declaration.
Known requests replay; unknown acknowledgements require the same request identity and body.
Reads never start work. Full artifact byte reads preserve original evidence.
`
