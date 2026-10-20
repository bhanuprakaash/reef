import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { GroupAvatar } from './creature';

const roles = ['lead', 'developer', 'tester'];

describe('GroupAvatar', () => {
  it('rings and animates only the working role', () => {
    const markup = renderToStaticMarkup(
      <GroupAvatar roles={roles} size={26} working="developer" />,
    );

    expect(markup.match(/c-ring/g)).toHaveLength(1);
    expect(markup.match(/c-dart/g)).toHaveLength(1);
    expect(markup).toContain('creature-live');
  });

  it('stays still when nobody is working', () => {
    const markup = renderToStaticMarkup(<GroupAvatar roles={roles} size={26} working={null} />);

    expect(markup).not.toContain('c-ring');
    expect(markup).not.toContain('creature-live');
  });

  it('animates a lone creature while it works', () => {
    const markup = renderToStaticMarkup(<GroupAvatar roles={['lead']} size={26} working="lead" />);

    expect(markup).toContain('c-dart');
    expect(markup).toContain('creature-live');
  });
});
