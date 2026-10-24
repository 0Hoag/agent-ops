// Sets the status:* label of a GitHub issue in one step: removes every other
// status:* label and adds the new one. GitHub labels are not exclusive, so adding
// `status:review` alone leaves the issue with two states.
//
//   node scripts/issue-status.mjs <issue> <status> [--dry-run]
//   node scripts/issue-status.mjs 12 "in progress"   # todo | in progress | review | blocked | done
//
// `done` closes the issue and removes all status:* labels. Needs `gh` (authenticated)
// run inside the repo, or `-R owner/repo` in GH_ARGS.
import { spawnSync } from 'node:child_process'

const STATUSES = {
    todo: 'status:todo',
    'in progress': 'status:in progress',
    review: 'status:review',
    blocked: 'status:blocked',
}

const args = process.argv.slice(2)
const dry = args.includes('--dry-run')
const [issue, ...rest] = args.filter((a) => a !== '--dry-run')
const status = rest.join(' ').trim().toLowerCase().replace(/[-_]+/g, ' ')
const done = status === 'done'

if (!/^\d+$/.test(issue ?? '') || (!done && !(status in STATUSES))) {
    console.error(
        `usage: node scripts/issue-status.mjs <issue number> <${[...Object.keys(STATUSES), 'done'].join(' | ')}> [--dry-run]`
    )
    process.exit(2)
}

const extra = (process.env.GH_ARGS ?? '').split(/\s+/).filter(Boolean)
const gh = (...a) => spawnSync('gh', [...a, ...extra], { encoding: 'utf8' })

const view = gh('issue', 'view', issue, '--json', 'labels')
if (view.status !== 0) {
    console.error(view.stderr || `gh could not read issue #${issue}`)
    process.exit(1)
}
const labels = (JSON.parse(view.stdout).labels ?? []).map((l) => l.name)
const current = labels.filter((l) => l.startsWith('status:'))
const target = done ? null : STATUSES[status]
const remove = current.filter((l) => l !== target)
const add = target && !current.includes(target) ? [target] : []

console.log(
    `#${issue}: [${current.join(', ') || 'no status'}] -> ${done ? 'closed, no status' : target}`
)
if (dry) process.exit(0)

const edit = ['issue', 'edit', issue]
if (remove.length) edit.push('--remove-label', remove.join(','))
if (add.length) edit.push('--add-label', add.join(','))
if (edit.length > 3) {
    const r = gh(...edit)
    if (r.status !== 0) {
        console.error(r.stderr)
        process.exit(1)
    }
}
if (done) {
    const r = gh('issue', 'close', issue)
    if (r.status !== 0) {
        console.error(r.stderr)
        process.exit(1)
    }
}
