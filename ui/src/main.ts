import { mount } from 'svelte';
import './lib/tokens/tokens.css';
import './lib/tokens/base.css';
import App from './App.svelte';

mount(App, { target: document.getElementById('app')! });
