import {afterEach,expect,it} from 'vitest'
import {cleanup,render,screen,within} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {ProjectSwimlane} from '@/components/iugum-project-swimlane'
afterEach(()=>{cleanup();localStorage.clear()})
function Rows({alpha=true}:{alpha?:boolean}){return <>{alpha&&<ProjectSwimlane project='alpha' count={2}><p>Alpha cards</p></ProjectSwimlane>}<ProjectSwimlane project='beta' count={3}><p>Beta cards</p></ProjectSwimlane></>}
it('collapses the entire project body independently and preserves focused header',async()=>{
 render(<Rows/>);const u=userEvent.setup()
 const alpha=screen.getByRole('button',{name:'Collapse project alpha'})
 expect(alpha).toHaveAttribute('aria-expanded','true')
 expect(alpha).toHaveTextContent('2 cards')
 const body=document.getElementById(alpha.getAttribute('aria-controls')!)!
 expect(within(body).getByText('Alpha cards')).toBeVisible()
 await u.click(alpha)
 expect(alpha).toHaveAttribute('aria-expanded','false')
 expect(screen.getByText('Alpha cards')).not.toBeVisible()
 expect(screen.getByText('Beta cards')).toBeVisible()
 expect(document.activeElement).toBe(alpha)
 await u.keyboard('{Enter}')
 expect(screen.getByText('Alpha cards')).toBeVisible()
 await u.keyboard(' ')
 expect(screen.getByText('Alpha cards')).not.toBeVisible()
})
it('keeps per-slug collapse choices across reload and project deselect/reselect',async()=>{
 const first=render(<Rows/>);const u=userEvent.setup()
 await u.click(screen.getByRole('button',{name:'Collapse project alpha'}))
 first.rerender(<Rows alpha={false}/>);first.rerender(<Rows/>);
 expect(screen.getByRole('button',{name:'Expand project alpha'})).toHaveAttribute('aria-expanded','false')
 expect(screen.getByText('Beta cards')).toBeVisible()
 first.unmount();render(<Rows/>);
 expect(screen.getByText('Alpha cards')).not.toBeVisible()
 expect(screen.getByText('Beta cards')).toBeVisible()
 await u.click(screen.getByRole('button',{name:'Expand project alpha'}))
 expect(screen.getByText('Alpha cards')).toBeVisible()
})
it('does not collide on encoded project slugs',async()=>{
 render(<><ProjectSwimlane project='a/b' count={1}>Slash</ProjectSwimlane><ProjectSwimlane project='a%2Fb' count={1}>Percent</ProjectSwimlane></>)
 await userEvent.setup().click(screen.getByRole('button',{name:'Collapse project a/b'}))
 expect(screen.getByText('Slash')).not.toBeVisible()
 expect(screen.getByText('Percent')).toBeVisible()
})
