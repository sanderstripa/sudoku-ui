(()=>{let t='light';try{t=localStorage.getItem('sudoku-theme')||localStorage.getItem('theme')||t}catch{}document.documentElement.dataset.theme=t==='dark'?'dark':'light'})();
