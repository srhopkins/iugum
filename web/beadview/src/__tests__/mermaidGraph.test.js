import {
  buildMermaidFromBeads,
  resolveMermaidSource,
  sanitizeMermaidFlowchart,
} from '../mermaidGraph';

describe('mermaidGraph', () => {
  it('sanitizes hyphenated node ids outside quotes', () => {
    const src = `flowchart TD
  iugum-dus["☐ iugum-dus: Dependency"]
  iugum-5gw["☐ iugum-5gw: Watcher"]
  iugum-dus --> iugum-5gw
`;
    const out = sanitizeMermaidFlowchart(src);
    expect(out).toContain('n_iugum_dus[');
    expect(out).toContain('n_iugum_5gw[');
    expect(out).toContain('n_iugum_dus --> n_iugum_5gw');
    expect(out).toContain('iugum-dus: Dependency');
  });

  it('builds edges from depends_on', () => {
    const mermaid = buildMermaidFromBeads(
      [
        {
          id: 'iugum-dus',
          title: 'Deps',
          status: 'open',
          depends_on: ['iugum-5gw'],
        },
        {
          id: 'iugum-5gw',
          title: 'Watcher',
          status: 'open',
          depends_on: [],
        },
      ],
      { root: 'iugum-dus' }
    );
    expect(mermaid).toContain('n_iugum_dus');
    expect(mermaid).toContain('n_iugum_5gw');
    expect(mermaid).toContain('-->');
  });

  it('prefers beads-built graph with edges over api text', () => {
    const beads = [
      {
        id: 'iugum-dus',
        title: 'Deps',
        status: 'open',
        depends_on: ['iugum-5gw'],
      },
      { id: 'iugum-5gw', title: 'Watcher', status: 'open', depends_on: [] },
    ];
    const api = `flowchart TD\n  iugum-ojz["alone"]\n`;
    const out = resolveMermaidSource(beads, api, { root: 'iugum-dus' });
    expect(out).toContain('n_iugum_dus');
    expect(out).not.toContain('ojz');
  });
});
