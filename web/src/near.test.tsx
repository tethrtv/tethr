import { act, render, screen } from '@testing-library/react'
import { expect, test, vi } from 'vitest'
import { useNear } from './near'

function Row({ name }: { name: string }) {
  const [ref, near] = useNear<HTMLLIElement>()
  return <li ref={ref}>{`${name} ${near ? 'near' : 'far'}`}</li>
}

test('waits until a row comes near the screen, then stays near', () => {
  let report: IntersectionObserverCallback = () => {}
  const watched = new Set<Element>()
  vi.stubGlobal(
    'IntersectionObserver',
    class {
      constructor(callback: IntersectionObserverCallback) {
        report = callback
      }
      observe(el: Element) {
        watched.add(el)
      }
      unobserve(el: Element) {
        watched.delete(el)
      }
    },
  )
  render(
    <ol>
      <Row name="Heat" />
      <Row name="Lola" />
    </ol>,
  )
  expect(screen.getByText('Heat far')).toBeTruthy()
  expect(watched.size).toBe(2)

  const heat = screen.getByText('Heat far')
  act(() => report([{ target: heat, isIntersecting: true } as unknown as IntersectionObserverEntry], {} as never))
  expect(screen.getByText('Heat near')).toBeTruthy()
  expect(screen.getByText('Lola far')).toBeTruthy()
  expect(watched.size).toBe(1)

  act(() => report([{ target: heat, isIntersecting: false } as unknown as IntersectionObserverEntry], {} as never))
  expect(screen.getByText('Heat near')).toBeTruthy()
  vi.unstubAllGlobals()
})
