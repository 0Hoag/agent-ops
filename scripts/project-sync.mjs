// Mirrors the issues of both repos into the GitHub Project "Agent Ops" (kanban + roadmap) and keeps one
// epic issue per wave with every wave issue attached as a sub-issue. Labels stay the source of truth;
// this script only copies them into Project fields. Idempotent: only differences are written.
//
//   node scripts/project-sync.mjs            # sync
//   node scripts/project-sync.mjs --dry-run  # print what would change
//
// Needs `gh` with the `project` scope (gh auth refresh -s project).
import { spawnSync } from 'node:child_process'
import { readFileSync } from 'node:fs'

const OWNER = '0Hoag'
const TITLE = 'Agent Ops'
const REPOS = ['agent-ops', 'agent-ops-client']
const EPIC_REPO = 'agent-ops'
const dry = process.argv.includes('--dry-run')

const STATUS = { todo: 'Todo', 'in progress': 'In progress', review: 'Review', blocked: 'Blocked' }
const FIELDS = {
    Status: ['Todo', 'In progress', 'Review', 'Blocked', 'Done'],
    Wave: ['WS', ...Array.from({ length: 18 }, (_, i) => `W${i}`)],
    Area: ['api', 'client', 'ci', 'docs'],
    Priority: ['P0', 'P1', 'P2', 'P3'],
}
const COLORS = ['GRAY', 'BLUE', 'YELLOW', 'RED', 'GREEN', 'PURPLE', 'ORANGE', 'PINK']

function gh(args, input) {
    const r = spawnSync('gh', args, { encoding: 'utf8', input, maxBuffer: 64 << 20 })
    if (r.status !== 0) throw new Error(`gh ${args.slice(0, 3).join(' ')}: ${r.stderr.trim()}`)
    return r.stdout
}
const gql = (query, variables = {}) => {
    const out = JSON.parse(gh(['api', 'graphql', '--input', '-'], JSON.stringify({ query, variables })))
    if (out.errors) throw new Error(JSON.stringify(out.errors))
    return out.data
}
const write = (what, fn) => (console.log(`${dry ? 'would ' : ''}${what}`), dry ? null : fn())

// --- project, visibility, linked repos ---
const owner = gql(`query($l:String!){user(login:$l){id projectsV2(first:50){nodes{id number title}}}}`, { l: OWNER }).user
let project = owner.projectsV2.nodes.find((p) => p.title === TITLE)
if (!project) {
    project = write(`create project "${TITLE}"`, () =>
        gql(`mutation($o:ID!,$t:String!){createProjectV2(input:{ownerId:$o,title:$t}){projectV2{id number}}}`,
            { o: owner.id, t: TITLE }).createProjectV2.projectV2)
    if (!project) process.exit(0)
    gql(`mutation($p:ID!){updateProjectV2(input:{projectId:$p,public:true}){projectV2{id}}}`, { p: project.id })
    for (const repo of REPOS) {
        const id = gql(`query($o:String!,$n:String!){repository(owner:$o,name:$n){id}}`, { o: OWNER, n: repo }).repository.id
        gql(`mutation($p:ID!,$r:ID!){linkProjectV2ToRepository(input:{projectId:$p,repositoryId:$r}){repository{id}}}`, { p: project.id, r: id })
    }
}

// --- fields ---
const FIELD_Q = `query($p:ID!){node(id:$p){... on ProjectV2{fields(first:50){nodes{
  ... on ProjectV2Field{id name dataType} ... on ProjectV2SingleSelectField{id name options{id name}}}}}}}`
const options = (names) => names.map((name, i) => ({ name, color: COLORS[i % COLORS.length], description: '' }))
let fields = gql(FIELD_Q, { p: project.id }).node.fields.nodes
for (const [name, names] of Object.entries(FIELDS)) {
    const f = fields.find((x) => x.name === name)
    if (!f) write(`create field ${name}`, () => gql(`mutation($p:ID!,$n:String!,$o:[ProjectV2SingleSelectFieldOptionInput!]){
      createProjectV2Field(input:{projectId:$p,dataType:SINGLE_SELECT,name:$n,singleSelectOptions:$o}){clientMutationId}}`,
        { p: project.id, n: name, o: options(names) }))
    else if (f.options.map((o) => o.name).join() !== names.join()) write(`set options of ${name}`, () =>
        gql(`mutation($f:ID!,$o:[ProjectV2SingleSelectFieldOptionInput!]){updateProjectV2Field(input:{fieldId:$f,singleSelectOptions:$o}){clientMutationId}}`,
            { f: f.id, o: options(names) }))
}
for (const name of ['Start date', 'Target date']) {
    if (!fields.find((x) => x.name === name)) write(`create field ${name}`, () =>
        gql(`mutation($p:ID!,$n:String!){createProjectV2Field(input:{projectId:$p,dataType:DATE,name:$n}){clientMutationId}}`, { p: project.id, n: name }))
}
fields = gql(FIELD_Q, { p: project.id }).node.fields.nodes
const field = (name) => fields.find((x) => x.name === name)

// --- issues (REST: node id for the Project, numeric id for sub-issues) ---
const issues = REPOS.flatMap((repo) => gh(['api', '--paginate', `repos/${OWNER}/${repo}/issues?state=all&per_page=100`,
    '--jq', '.[] | select(.pull_request | not) | {id, node_id, number, title, state, created_at, closed_at, labels: [.labels[].name]}'])
    .trim().split('\n').filter(Boolean).map((l) => ({ ...JSON.parse(l), repo })))
const label = (i, prefix) => i.labels.find((l) => l.startsWith(prefix))?.slice(prefix.length)

// --- epics: one per wave, every wave issue attached as a sub-issue ---
const waveNames = Object.fromEntries([...readFileSync(new URL('../ROADMAP.md', import.meta.url), 'utf8')
    .matchAll(/^### (W\d+|WS): (.+)$/gm)].map((m) => [m[1], m[2].trim()]))
const isEpic = (i) => i.labels.includes('type:epic')
for (const wave of FIELDS.Wave) {
    const members = issues.filter((i) => !isEpic(i) && label(i, 'wave:') === wave)
    if (!members.length) continue
    let epic = issues.find((i) => isEpic(i) && label(i, 'wave:') === wave)
    if (!epic) {
        write(`create epic ${wave}`, () => {
            for (const l of ['type:epic', `wave:${wave}`]) gh(['label', 'create', l, '-R', `${OWNER}/${EPIC_REPO}`, '--force'])
            const url = gh(['issue', 'create', '-R', `${OWNER}/${EPIC_REPO}`, '-t', `Epic ${wave}: ${waveNames[wave] ?? wave}`,
                '-b', `Tracks every \`wave:${wave}\` issue (sub-issues). Maintained by scripts/project-sync.mjs.`,
                '-l', `type:epic,wave:${wave}`]).trim()
            const n = url.split('/').pop()
            epic = { ...JSON.parse(gh(['api', `repos/${OWNER}/${EPIC_REPO}/issues/${n}`, '--jq',
                '{id, node_id, number, title, state, created_at, closed_at, labels: [.labels[].name]}'])), repo: EPIC_REPO }
            issues.push(epic)
        })
        if (!epic) continue
    }
    const linked = new Set(gh(['api', '--paginate', `repos/${OWNER}/${EPIC_REPO}/issues/${epic.number}/sub_issues`, '--jq', '.[].id'])
        .trim().split('\n').filter(Boolean).map(Number))
    for (const m of members.filter((m) => !linked.has(m.id))) {
        try {
            write(`attach ${m.repo}#${m.number} to epic ${wave}`, () => gh(['api', '-X', 'POST',
                `repos/${OWNER}/${EPIC_REPO}/issues/${epic.number}/sub_issues`, '-F', `sub_issue_id=${m.id}`]))
        } catch (e) { console.warn(`  skip ${m.repo}#${m.number}: ${e.message.split('\n')[0]}`) }
    }
    epic.children = members
    const allClosed = members.every((m) => m.state === 'closed')
    if (allClosed !== (epic.state === 'closed')) write(`${allClosed ? 'close' : 'reopen'} epic ${wave}`, () => {
        gh(['issue', allClosed ? 'close' : 'reopen', String(epic.number), '-R', `${OWNER}/${EPIC_REPO}`])
        epic.state = allClosed ? 'closed' : 'open'
        epic.closed_at = allClosed ? new Date().toISOString() : null
    })
}

// --- project items and field values ---
const items = new Map()
for (let after = null; ;) {
    const page = gql(`query($p:ID!,$a:String){node(id:$p){... on ProjectV2{items(first:100,after:$a){pageInfo{hasNextPage endCursor}
      nodes{id content{... on Issue{id}} fieldValues(first:20){nodes{
        ... on ProjectV2ItemFieldSingleSelectValue{name field{... on ProjectV2SingleSelectField{name}}}
        ... on ProjectV2ItemFieldDateValue{date field{... on ProjectV2Field{name}}}}}}}}}}`, { p: project.id, a: after }).node.items
    for (const n of page.nodes) if (n.content?.id) items.set(n.content.id, {
        id: n.id, values: Object.fromEntries(n.fieldValues.nodes.filter((v) => v.field).map((v) => [v.field.name, v.name ?? v.date])) })
    if (!page.pageInfo.hasNextPage) break
    after = page.pageInfo.endCursor
}

const day = (s) => s?.slice(0, 10)
const desired = (i) => {
    const kids = i.children ?? []
    return {
        Status: i.state === 'closed' ? 'Done' : STATUS[label(i, 'status:')] ?? 'Todo',
        Wave: label(i, 'wave:'),
        Area: label(i, 'area:'),
        Priority: label(i, 'priority:')?.toUpperCase(),
        'Start date': kids.length ? kids.map((k) => day(k.created_at)).sort()[0] : day(i.created_at),
        'Target date': kids.length ? (kids.every((k) => k.closed_at) ? kids.map((k) => day(k.closed_at)).sort().pop() : undefined)
            : day(i.closed_at),
    }
}
let changes = 0
for (const issue of issues) {
    let item = items.get(issue.node_id)
    if (!item) {
        const id = write(`add ${issue.repo}#${issue.number}`, () => gql(`mutation($p:ID!,$c:ID!){addProjectV2ItemById(input:{projectId:$p,contentId:$c}){item{id}}}`,
            { p: project.id, c: issue.node_id }).addProjectV2ItemById.item.id)
        if (!id) continue
        item = { id, values: {} }
    }
    for (const [name, value] of Object.entries(desired(issue))) {
        if ((item.values[name] ?? undefined) === value) continue
        const f = field(name)
        changes++
        if (value === undefined) {
            write(`clear ${name} on ${issue.repo}#${issue.number}`, () => gql(`mutation($p:ID!,$i:ID!,$f:ID!){clearProjectV2ItemFieldValue(input:{projectId:$p,itemId:$i,fieldId:$f}){clientMutationId}}`,
                { p: project.id, i: item.id, f: f.id }))
            continue
        }
        const v = f.options ? { singleSelectOptionId: f.options.find((o) => o.name === value)?.id } : { date: value }
        if (f.options && !v.singleSelectOptionId) { console.warn(`  unknown ${name}=${value} on ${issue.repo}#${issue.number}`); continue }
        write(`set ${name}=${value} on ${issue.repo}#${issue.number}`, () => gql(`mutation($p:ID!,$i:ID!,$f:ID!,$v:ProjectV2FieldValue!){
          updateProjectV2ItemFieldValue(input:{projectId:$p,itemId:$i,fieldId:$f,value:$v}){clientMutationId}}`,
            { p: project.id, i: item.id, f: f.id, v }))
    }
}
console.log(`project #${project.number}: ${issues.length} issues, ${changes} field changes${dry ? ' (dry run)' : ''}`)
console.log(`https://github.com/users/${OWNER}/projects/${project.number}`)
