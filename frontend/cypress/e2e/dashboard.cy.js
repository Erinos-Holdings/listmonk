describe('Dashboard', () => {
  it('Opens dashboard', () => {
    cy.resetDB();
    cy.loginAndVisit('/');

    // List counts.
    cy.get('[data-cy=lists] .title').contains('2');
    cy.get('[data-cy=lists]')
      .and('contain', '1 Public')
      .and('contain', '1 Private')
      .and('contain', '1 Single opt-in')
      .and('contain', '1 Double opt-in');

    // Campaign counts.
    cy.get('[data-cy=campaigns] .title').contains('1');
    cy.get('[data-cy=campaigns-draft]').contains('1');

    // Subscriber counts. Fork (dashboard counts, integrations DASHBOARD-COUNTS-SPEC D1/D2): the
    // subscriptions of the public lists -- the install's one public list is the double opt-in
    // "Opt-in list", with anon@ unconfirmed on it (Pending).
    // Revision 2 (R1): Held, Unsubscribed and Blocklisted read rate-first over the total.
    cy.get('[data-cy=subscribers] .title').contains('1');
    cy.get('[data-cy=subscribers]')
      .should('contain', '0 Active')
      .and('contain', '0.0% (0) Held')
      .and('contain', '0.0% (0) Unsubscribed')
      .and('contain', '0.0% (0) Blocklisted')
      .and('contain', '1 Pending')
      .and('contain', '0 Orphans');

    // The +/- control expands the segment x language table and collapses it again.
    cy.get('[data-cy=subscribers-expand]').click();
    cy.get('[data-cy=subscribers-grid]').should('exist');
    cy.get('[data-cy=subscribers-expand]').click();
    cy.get('[data-cy=subscribers-grid]').should('not.exist');

    // Orphans stays a line when expanded (R3).
    cy.get('[data-cy=subscribers-expand]').click();
    cy.get('[data-cy=subscribers-orphans]').should('contain', '0 Orphans');
    cy.get('[data-cy=subscribers-expand]').click();

    // Message count, with Views, Clicks, Bounces and Complaints (D4, R2); nothing sent, so the
    // counts stand alone (R1).
    cy.get('[data-cy=messages] .title').contains('0');
    cy.get('[data-cy=messages]')
      .should('contain', '0 Views')
      .and('contain', '0 Clicks')
      .and('contain', '0 Bounces')
      .and('contain', '0 Complaints');

    // The Location panel (R4).
    cy.get('[data-cy=countries]').should('contain', 'Location');
  });
});
