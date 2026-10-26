const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const {test} = require('node:test');
const {parseComponent} = require('vue-template-compiler');
const parser = require('babel-eslint');

// Execute the component's real method body, with network/browser dependencies
// replaced by test doubles. No DOM or extra test framework is required.
function method(file, group, name, globals = {}) {
    const content = fs.readFileSync(path.join(__dirname, '../src', file), 'utf8');
    const script = parseComponent(content).script.content;
    const ast = parser.parse(script, {sourceType: 'module', range: true});
    const component = ast.body.find(node => node.type === 'ExportDefaultDeclaration').declaration;
    const section = component.properties.find(node => node.key.name === group).value;
    const entry = section.properties.find(node => node.key.name === name);
    return vm.runInNewContext(`({${script.slice(...entry.range)}}).${name}`, globals);
}

function summaryState() {
    return {
        code: 'T1', projectCodeCurrent: 'P1', taskSummaryRequestId: 0,
        taskSummaryData: null, taskSummaryModal: {loading: false},
        $message: {error(message) { this.messages.push(message); }, messages: []},
    };
}

test('task summary displays a successful response and stops loading', async () => {
    const generate = method('components/project/taskDetail.vue', 'methods', 'generateTaskSummary', {
        generateTaskSummaryReport: async args => ({data: {markdown: args.taskCode}}),
        checkResponse: () => true,
    });
    const state = summaryState();
    await generate.call(state);
    assert.equal(state.taskSummaryData.markdown, 'T1');
    assert.equal(state.taskSummaryModal.loading, false);
});

test('an old task response cannot overwrite the current task or its loading state', async () => {
    let resolve;
    const generate = method('components/project/taskDetail.vue', 'methods', 'generateTaskSummary', {
        generateTaskSummaryReport: () => new Promise(done => { resolve = done; }),
        checkResponse: () => true,
    });
    const state = summaryState();
    const pending = generate.call(state);
    assert.equal(generate.call(state), undefined, 'duplicate requests must be ignored');
    state.taskSummaryRequestId++;
    state.code = 'T2';
    resolve({data: {markdown: 'old task'}});
    await pending;
    assert.equal(state.taskSummaryData, null);
    assert.equal(state.taskSummaryModal.loading, true);
});

test('a failed summary request reports the error and allows retry', async () => {
    const generate = method('components/project/taskDetail.vue', 'methods', 'generateTaskSummary', {
        generateTaskSummaryReport: async () => { throw new Error('offline'); },
        checkResponse: () => true,
    });
    const state = summaryState();
    await generate.call(state);
    assert.equal(state.$message.messages.length, 1);
    assert.equal(state.taskSummaryModal.loading, false);
    assert.equal(state.taskSummaryData, null);
});

test('footer sends only when its websocket is open', () => {
    const click = method('views/index.vue', 'methods', 'footerClick', {WebSocket: {OPEN: 1}});
    const sent = [];
    const state = {$store: {state: {userInfo: {id: 7}}}};
    click.call(state);
    state.$websocket = {readyState: 0, send: value => sent.push(value)};
    click.call(state);
    assert.equal(sent.length, 0);
    state.$websocket.readyState = 1;
    click.call(state);
    assert.equal(sent[0], JSON.stringify({uid: 7}));
});
