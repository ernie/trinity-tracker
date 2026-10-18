export function AboutSection() {
  return (
    <section className="landing-section landing-about">
      <div className="landing-about__ornament" aria-hidden>
        <span className="landing-about__rule" />
        <span>✦</span>
        <span className="landing-about__rule" />
      </div>
      <div className="landing-about__prose">
        <p>
          Trinity is a community for people who think the best arena shooter
          ever made came out in 1999.
        </p>
        <p>
          It&rsquo;s Quake III Arena the way you remember it, which isn&rsquo;t
          quite how it was. If you&rsquo;re new to it, it&rsquo;s Quake III the
          way you&rsquo;d expect a game to work today: widescreen support,
          high-resolution UI, a better renderer, settings menus that offer the
          options that matter, and VR as a first-class citizen, with flatscreen
          and VR players in the same matches.
        </p>
        <p>
          Long-standing bugs and unfinished pieces are fixed and finished,
          especially in Team Arena, and the grappling hook id never completed is
          now the grapple. Blood and damage effects are improved, and a long
          tail of small quality-of-life changes runs through the menus and the
          game.
        </p>
        <p>
          This site is the tracker. Every match is recorded, so you can watch
          fights live or replay them later. Playing needs your own copy of Quake
          III Arena; watching needs nothing.
        </p>
      </div>
    </section>
  );
}
