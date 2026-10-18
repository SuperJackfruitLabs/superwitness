// Once: carry a light/dark choice made with the old toggle into vibekit's appearance cookie.
(function(){try{if(!/(?:^|; )vk_appearance=/.test(document.cookie)){var t=localStorage.getItem('superwitness.theme');if(t==='light'||t==='dark'){document.cookie='vk_appearance=daylight.'+t+'.strong.0; Path=/; Max-Age=31536000; SameSite=Lax'+(location.protocol==='https:'?'; Secure':'');}}}catch(_){}})();
